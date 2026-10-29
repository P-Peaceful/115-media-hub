package drive115

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"time"
)

type qrSession struct {
	UID    string      `json:"uid"`
	Time   wireInteger `json:"time"`
	Sign   string      `json:"sign"`
	device Device
}

func loginApp(device Device) (string, bool) {
	switch device {
	case DeviceWeb, DeviceAlipayMini, DeviceWechatMini:
		return string(device), true
	default:
		return "", false
	}
}
func (d *Driver) StartQRCode(ctx context.Context, device Device) (Challenge, error) {
	if _, ok := loginApp(device); !ok {
		return Challenge{}, ErrInvalidDevice
	}
	var response struct {
		Data qrSession `json:"data"`
	}
	if err := d.getJSON(ctx, "https://qrcodeapi.115.com/api/1.0/web/1.0/token/", nil, Credential{}, &response); err != nil {
		return Challenge{}, err
	}
	session := response.Data
	if session.UID == "" || session.Time == "" || session.Sign == "" {
		return Challenge{}, protocolError()
	}
	session.device = device
	image, err := d.request(ctx, http.MethodGet, "https://qrcodeapi.115.com/api/1.0/mac/1.0/qrcode", url.Values{"uid": {session.UID}}, Credential{})
	if err != nil {
		return Challenge{}, err
	}
	if !bytes.HasPrefix(image, []byte("\x89PNG\r\n\x1a\n")) {
		return Challenge{}, protocolError()
	}
	return Challenge{opaque: &session, Image: image, ExpiresAt: time.Now().UTC().Add(5 * time.Minute)}, nil
}
func (d *Driver) PollQRCode(ctx context.Context, ch Challenge) (Status, error) {
	if ch.opaque == nil {
		return "", ErrInvalidChallenge
	}
	s := ch.opaque
	var response struct {
		Data struct {
			Status *wireInteger `json:"status"`
		} `json:"data"`
	}
	if err := d.getJSON(ctx, "https://qrcodeapi.115.com/get/status/", url.Values{"uid": {s.UID}, "time": {string(s.Time)}, "sign": {s.Sign}}, Credential{}, &response); err != nil {
		return "", err
	}
	if response.Data.Status == nil {
		return "", ErrUnknownStatus
	}
	switch *response.Data.Status {
	case "0":
		return Waiting, nil
	case "1":
		return Scanned, nil
	case "2":
		return Allowed, nil
	case "-1":
		return Expired, nil
	case "-2":
		return Canceled, nil
	default:
		return "", ErrUnknownStatus
	}
}
func (d *Driver) CompleteQRCode(ctx context.Context, ch Challenge, device Device) (Credential, error) {
	app, ok := loginApp(device)
	if !ok {
		return Credential{}, ErrInvalidDevice
	}
	if ch.opaque == nil || ch.opaque.device != device {
		return Credential{}, ErrInvalidChallenge
	}
	data, err := d.request(ctx, http.MethodPost, "https://qrcodeapi.115.com/app/1.0/"+app+"/1.0/login/qrcode/", url.Values{"account": {ch.opaque.UID}}, Credential{})
	if err != nil {
		return Credential{}, err
	}
	var response struct {
		Data struct {
			Cookie Credential `json:"cookie"`
		} `json:"data"`
	}
	if err := decodeResponse(data, &response); err != nil {
		return Credential{}, redactError(err, url.Values{"account": {ch.opaque.UID}, "sign": {ch.opaque.Sign}, "time": {string(ch.opaque.Time)}}, Credential{})
	}
	cr := response.Data.Cookie
	if cr.UID == "" || cr.CID == "" || cr.SEID == "" || cr.KID == "" {
		return Credential{}, protocolError()
	}
	return cr, nil
}
