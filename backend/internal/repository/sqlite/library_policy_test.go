// library_policy_test.go：验证已有 STRM 策略的旧配置读取默认值和显式非法值处理，不允许读取隐式回写。

package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestReadPrePolicyLibrariesDefaultsOnlyMissingPolicyWithoutWriting 验证仅缺失字段补 skip，列表与详情一致且 JSON、修订号及时间均不变。
func TestReadPrePolicyLibrariesDefaultsOnlyMissingPolicyWithoutWriting(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "pre-policy.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	config := validLibraryConfig("/media")
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	prePolicyJSON := strings.Replace(string(encoded), `"strm_existing_file_policy":"skip",`, "", 1)
	if prePolicyJSON == string(encoded) {
		t.Fatal("fixture still has existing-file policy")
	}
	stamp := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	for _, model := range []libraryModel{
		{ID: "1", ConfigJSON: prePolicyJSON, Revision: 7, CreatedAt: stamp, UpdatedAt: stamp},
		{ID: "2", ConfigJSON: strings.Replace(string(encoded), `"skip"`, `"overwrite"`, 1), Revision: 3, CreatedAt: stamp, UpdatedAt: stamp},
	} {
		if err = db.orm.Create(&model).Error; err != nil {
			t.Fatal(err)
		}
	}
	var before []libraryModel
	if err = db.orm.Order("id ASC").Find(&before).Error; err != nil {
		t.Fatal(err)
	}
	items, err := db.ListLibraries(ctx, "", 10)
	if err != nil || len(items) != 2 {
		t.Fatalf("pre-policy list: %+v %v", items, err)
	}
	if items[0].Config != config || items[0].Revision != 7 || items[1].STRMExistingFilePolicy != "overwrite" {
		t.Fatalf("unexpected list defaults/config: %+v", items)
	}
	for _, listed := range items {
		detail, getErr := db.GetLibrary(ctx, listed.ID)
		if getErr != nil || !reflect.DeepEqual(detail, listed) {
			t.Fatalf("detail differs from list: %+v %v", detail, getErr)
		}
	}
	var after []libraryModel
	if err = db.orm.Order("id ASC").Find(&after).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("reading configuration mutated stored JSON, revision or timestamps")
	}
}

// TestStoredExplicitInvalidPolicyIsNotDefaulted 验证空值、错误类型和未知策略作为损坏拒绝，不被默认值掩盖。
func TestStoredExplicitInvalidPolicyIsNotDefaulted(t *testing.T) {
	for _, value := range []string{`""`, `null`, `"rename"`, `true`, `123`} {
		t.Run(value, func(t *testing.T) {
			_, err := libraryFromModel(libraryModel{
				ConfigJSON: `{"strm_existing_file_policy":` + value + `}`,
			})
			if err == nil {
				t.Fatal("invalid explicit policy silently accepted/defaulted")
			}
		})
	}
}
