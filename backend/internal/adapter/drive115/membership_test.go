package drive115

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestMembershipNameComesVerbatimFromOptionalCookieEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		status           int
	}{
		{"literal", `{"state":true,"data":{"user_limit":{"level_name":"长期尊享 & <新会员>"}}}`, "长期尊享 & <新会员>", 200},
		{"unfamiliar", `{"state":true,"data":{"user_limit":{"level_name":"  新名称·未来版本  "}}}`, "  新名称·未来版本  ", 200},
		{"missing", `{"state":true,"data":{"user_limit":{}}}`, "", 200},
		{"blank", `{"state":true,"data":{"user_limit":{"level_name":"  "}}}`, "", 200},
		{"null", `{"state":true,"data":{"user_limit":{"level_name":null}}}`, "", 200},
		{"number", `{"state":true,"data":{"user_limit":{"level_name":2}}}`, "", 200},
		{"malformed", `not json`, "", 200},
		{"failed", `{"state":false,"error":"unavailable"}`, "", 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			d := testDriver(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/":
					w.Write([]byte(`{"state":true,"data":{"user_id":123,"user_name":"account","vip":8}}`))
				case "/files":
					w.Write([]byte(`{"state":true,"cid":0,"count":0,"data":[]}`))
				case "/files/index_info":
					w.Write([]byte(`{"state":true,"data":{}}`))
				case "/user/vip_limit":
					called = true
					if r.URL.Query().Get("feature") != "2" {
						t.Error("missing feature=2")
					}
					w.WriteHeader(tc.status)
					w.Write([]byte(tc.body))
				default:
					t.Errorf("unexpected endpoint %s", r.URL.Path)
				}
			})
			evidence, err := d.VerifyCredential(context.Background(), Credential{})
			if err != nil || !evidence.RootReadable || evidence.AccountID != "123" {
				t.Fatalf("lost identity %+v %v", evidence, err)
			}
			data, _ := json.Marshal(evidence.Profile)
			var profile map[string]any
			json.Unmarshal(data, &profile)
			got, _ := profile["membershipName"].(string)
			if !called || got != tc.want {
				t.Fatalf("membership name %q want %q; queried=%v", got, tc.want, called)
			}
			if _, ok := profile["membershipLevel"]; ok {
				t.Fatal("numeric membership grade exposed")
			}
			if evidence.Profile.MembershipStatus != "unknown" {
				t.Fatalf("numeric grade inferred status: %s", evidence.Profile.MembershipStatus)
			}
		})
	}
}
