// download_link.go：负责 Cookie 取链通道编排、各端点的协议编解码及可信下载目标核验。

package provider115

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/SheltonZhu/115driver/pkg/crypto/m115"

	adapter "github.com/wenzhengcheng/115-media-hub/backend/internal/adapter/provider115"
)

const (
	chromeDownloadURL    = "https://proapi.115.com/app/chrome/downurl"
	androidDownloadURL   = "https://proapi.115.com/android/2.0/ufile/download"
	androidDownloadAlt   = "https://pro.api.115.com/android/2.0/ufile/download"
	webDownloadURL       = "https://webapi.115.com/files/download"
	downloadLinkMaxBytes = 64 << 10
)

type downloadProtocol string

const (
	androidPrimary downloadProtocol = "android_primary"
	androidMirror  downloadProtocol = "android_mirror"
	chromeLink     downloadProtocol = "chrome"
	webLinkPost    downloadProtocol = "web_post"
	webLinkGet     downloadProtocol = "web_get"
)

type downloadEndpoint struct {
	protocol downloadProtocol
	address  string
}

// downloadTarget 只在内存中传递临时地址及签发材料，禁止串入可持久化结构、日志和前端响应。
type downloadTarget struct {
	URL       string
	UserAgent string
	Cookies   []*http.Cookie
}

// downloadIdentity 是调用前已绑定账号、文件 ID 与定位码的源标识。
type downloadIdentity struct{ id, code string }

// downloadEndpoints 固定 Cookie 通道的优先顺序，每种协议只使用其已知的 HTTPS 端点。
func downloadEndpoints() []downloadEndpoint {
	return []downloadEndpoint{
		{androidPrimary, androidDownloadURL},
		{androidMirror, androidDownloadAlt},
		{chromeLink, chromeDownloadURL},
		{webLinkPost, webDownloadURL},
		{webLinkGet, webDownloadURL},
	}
}

// resolveDownloadTarget 按已审查的固定顺序尝试取链，明确的账号、文件、限流和安全错误立即结束。
func (s *Service) resolveDownloadTarget(ctx context.Context, cookie, sourceFileID, playbackCode, userAgent string, operation adapter.Operation, priority adapter.Priority) (downloadTarget, error) {
	endpoints := s.downloadLinks
	identity := downloadIdentity{sourceFileID, playbackCode}
	var lastErr, responseErr error
	for i, endpoint := range endpoints {
		target, err := s.requestDownloadLink(ctx, endpoint, identity, cookie, userAgent, operation, priority)
		if err == nil {
			return target, nil
		}
		if !mayFallbackDownloadLink(err) {
			return downloadTarget{}, err
		}
		if errors.Is(err, ErrDownloadLinkResponse) {
			responseErr = err
		}
		lastErr = err
		if i+1 < len(endpoints) {
			s.logger.DebugContext(ctx, "115 take-link fallback", "operation", string(operation), "channel", string(endpoint.protocol), "next_channel", string(endpoints[i+1].protocol), "attempt", i+1)
		}
	}
	if responseErr != nil {
		return downloadTarget{}, responseErr
	}
	return downloadTarget{}, errors.Join(ErrDownloadTakeLink, lastErr)
}

// mayFallbackDownloadLink 仅允许接口协议或网络问题切换端点，凭据复查错误有 cause 且不回退。
func mayFallbackDownloadLink(err error) bool {
	if errors.Is(err, ErrDownloadLinkResponse) {
		return true
	}
	var upstream *adapter.Error
	if !errors.As(err, &upstream) || upstream.Unwrap() != nil {
		return false
	}
	return upstream.Kind == adapter.Protocol || upstream.Kind == adapter.Temporary ||
		(upstream.Kind == adapter.NotFound && !errors.Is(err, ErrSourceNotFound))
}

// requestDownloadLink 对一个固定端点发送一次受限请求，并获取已复查凭据的响应 Cookie。
func (s *Service) requestDownloadLink(ctx context.Context, endpoint downloadEndpoint, identity downloadIdentity, cookie, userAgent string, operation adapter.Operation, priority adapter.Priority) (downloadTarget, error) {
	address, body, key, err := endpoint.encode(identity.code, s.now().Unix())
	if err != nil {
		return downloadTarget{}, err
	}
	method := http.MethodPost
	if endpoint.protocol == webLinkGet {
		method = http.MethodGet
	}
	target := downloadTarget{UserAgent: userAgent}
	r := adapter.Request{
		Scope: accountScope(cookie), Operation: operation, Priority: priority, Method: method,
		URL: address, Header: requestHeaders(cookie, userAgent), Body: body, MaxBytes: downloadLinkMaxBytes,
		Check: s.checkCredential(cookie), Inspect: s.inspectBusiness(cookie), SingleDownloadAttempt: true,
	}
	r.DownloadCookies = func(cookies []*http.Cookie) { target.Cookies = cookies }
	data, err := s.doRequest(ctx, r)
	if err != nil {
		// Chrome 的 404 历史上表示文件不存在；新路径的 HTTP 404 可能是接口未部署，文件缺失由业务码确认。
		missing := ErrUpstream
		if endpoint.protocol == chromeLink {
			missing = ErrSourceNotFound
		}
		return downloadTarget{}, mapRequestError(err, missing)
	}
	target.URL, err = endpoint.parse(data, identity, func(encoded string) ([]byte, error) {
		return decodePlaybackData(encoded, key)
	})
	if err != nil {
		return downloadTarget{}, err
	}
	if operation == adapter.DownloadLink && !validDownloadURL(target.URL) {
		return downloadTarget{}, errors.Join(ErrDownloadUntrustedTarget, ErrUpstream)
	}
	if operation == adapter.Playback && !target.permitsPlaybackRedirect() {
		return downloadTarget{}, errors.Join(ErrDownloadLinkResponse, ErrUpstream)
	}
	return target, nil
}

// permitsPlaybackRedirect 拒绝必须带签发 Cookie 的地址，普通 302 无法为其他站点设置 Cookie。
func (target downloadTarget) permitsPlaybackRedirect() bool {
	u, err := url.Parse(target.URL)
	if err != nil {
		return false
	}
	switch u.Query().Get("f") {
	case "1", "2":
		return true
	case "3":
		return false
	default:
		return len(target.Cookies) == 0
	}
}

// encode 依据端点使用字段 pick_code 或 pickcode，仅 Chrome 兼容接口使用原有时间参数。
func (e downloadEndpoint) encode(pickcode string, now int64) (string, []byte, m115.Key, error) {
	if e.protocol == webLinkGet {
		return e.address + "?" + url.Values{"pickcode": {pickcode}}.Encode(), nil, m115.Key{}, nil
	}
	address := e.address
	if e.protocol == chromeLink {
		address += "?" + url.Values{"t": {strconv.FormatInt(now, 10)}}.Encode()
	}
	payload, err := e.protocol.locatorPayload(pickcode)
	if err != nil {
		return "", nil, m115.Key{}, ErrUpstream
	}
	key := m115.GenerateKey()
	return address, []byte(url.Values{"data": {m115.Encode(payload, key)}}.Encode()), key, nil
}

// locatorPayload 选择 m115 载荷中的定位码字段。Android 协议与 Chrome、Web POST 使用不同拼法。
func (p downloadProtocol) locatorPayload(pickcode string) ([]byte, error) {
	field := "pickcode"
	if p == androidPrimary || p == androidMirror {
		field = "pick_code"
	}
	return json.Marshal(map[string]string{field: pickcode})
}

type downloadEnvelope struct {
	State   *upstreamState  `json:"state"`
	Code    json.RawMessage `json:"errno"`
	ErrCode json.RawMessage `json:"errcode"`
	ErrNo   json.RawMessage `json:"errNo"`
	Error   string          `json:"error"`
	Message string          `json:"msg"`
	Data    json.RawMessage `json:"data"`
}

// parse 先识别错误信封，再按当前通道读取明文 URL 或解码加密数据。
func (e downloadEndpoint) parse(data []byte, identity downloadIdentity, decode func(string) ([]byte, error)) (string, error) {
	var envelope downloadEnvelope
	if json.Unmarshal(data, &envelope) != nil || envelope.State == nil {
		return "", errors.Join(ErrDownloadLinkResponse, ErrUpstream)
	}
	if !bool(*envelope.State) {
		fallback := envelope.ErrCode
		if parseInteger(fallback) == nil {
			fallback = envelope.ErrNo
		}
		return "", playbackUpstreamError(envelope.Code, fallback, envelope.Error, envelope.Message)
	}
	if e.protocol == webLinkGet {
		return webDownloadTarget(data, envelope.Data, identity)
	}
	var encoded string
	if json.Unmarshal(envelope.Data, &encoded) != nil || encoded == "" {
		return "", errors.Join(ErrDownloadLinkResponse, ErrUpstream)
	}
	decoded, err := decode(encoded)
	if err != nil {
		return "", errors.Join(ErrDownloadLinkResponse, err)
	}
	if e.protocol == chromeLink {
		return playbackTarget(decoded, identity.id, identity.code)
	}
	return singleDownloadTarget(decoded, identity, e.protocol == webLinkPost)
}

// decodePlaybackData 解码上游下载协议，并将第三方解码器处理畸形数据的 panic 收敛为上游错误。
func decodePlaybackData(encoded string, key m115.Key) (_ []byte, resultErr error) {
	// 第三方解码器假设解包后至少有一个密钥头，畸形响应不能导致进程崩溃。
	defer func() {
		if recover() != nil {
			resultErr = ErrUpstream
		}
	}()
	decoded, err := m115.Decode(encoded, key)
	if err != nil {
		return nil, ErrUpstream
	}
	return decoded, nil
}

// downloadLinkFields 只读取已知字段，防止把任意嵌套 URL 误识别为下载目标。
type downloadLinkFields struct {
	FileID   json.RawMessage `json:"file_id"`
	FID      json.RawMessage `json:"fid"`
	PickCode json.RawMessage `json:"pick_code"`
	Pick     json.RawMessage `json:"pickcode"`
	URL      json.RawMessage `json:"url"`
	FileURL  string          `json:"file_url"`
}

// verify 检查响应回显的 ID 或定位码；未回显时连接仍绑定发出请求所用的定位码。
func (f downloadLinkFields) verify(identity downloadIdentity) error {
	for _, pair := range [...]struct {
		value json.RawMessage
		want  string
	}{{f.FileID, identity.id}, {f.FID, identity.id}, {f.PickCode, identity.code}, {f.Pick, identity.code}} {
		if len(pair.value) == 0 || string(pair.value) == "null" {
			continue
		}
		var actual string
		if json.Unmarshal(pair.value, &actual) != nil {
			actual = string(pair.value)
		}
		if actual != pair.want {
			return ErrSourceNotFound
		}
	}
	return nil
}

// singleDownloadTarget 从 Android 或 Web POST 的单文件对象读取 URL 并核对任何身份回显。
func singleDownloadTarget(data []byte, identity downloadIdentity, fileURL bool) (string, error) {
	var response downloadLinkFields
	if json.Unmarshal(data, &response) != nil {
		return "", errors.Join(ErrDownloadLinkResponse, ErrUpstream)
	}
	if err := response.verify(identity); err != nil {
		return "", err
	}
	if fileURL && response.FileURL != "" {
		return checkedDownloadTarget(response.FileURL)
	}
	return checkedDownloadTarget(downloadURLField(response.URL))
}

// webDownloadTarget 兼容 GET 信封的 url、data.url 和 file_url，核对两层返回的源身份。
func webDownloadTarget(data, inner []byte, identity downloadIdentity) (string, error) {
	var top, nested downloadLinkFields
	if json.Unmarshal(data, &top) != nil || (len(inner) != 0 && json.Unmarshal(inner, &nested) != nil) {
		return "", errors.Join(ErrDownloadLinkResponse, ErrUpstream)
	}
	if err := top.verify(identity); err != nil {
		return "", err
	}
	if err := nested.verify(identity); err != nil {
		return "", err
	}
	for _, target := range [...]string{downloadURLField(top.URL), downloadURLField(nested.URL), top.FileURL, nested.FileURL} {
		if target != "" {
			return checkedDownloadTarget(target)
		}
	}
	return checkedDownloadTarget("")
}

// downloadURLField 只解读直接 URL 和一层 url 对象这两种上游已知形态。
func downloadURLField(raw json.RawMessage) string {
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		return plain
	}
	var nested struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(raw, &nested) == nil {
		return nested.URL
	}
	return ""
}

// checkedDownloadTarget 区分接口未给 URL 与返回不可信目标；后者禁止改走其他端点。
func checkedDownloadTarget(target string) (string, error) {
	if target == "" {
		return "", errors.Join(ErrDownloadLinkResponse, ErrUpstream)
	}
	if !validPlaybackURL(target) {
		return "", errors.Join(ErrDownloadUntrustedTarget, ErrUpstream)
	}
	return target, nil
}
