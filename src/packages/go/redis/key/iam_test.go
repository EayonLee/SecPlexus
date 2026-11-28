package key

import "testing"

func TestTemplatesKeepModuleAndBusinessBoundaries(t *testing.T) {
	if IAMLoginIP != "spx:iam:ratelimit:ip:%s" || IAMLoginAccount != "spx:iam:ratelimit:account:%s" {
		t.Fatal("模板必须直接包含默认 spx 前缀")
	}

	if IAMLoginIP.Format("digest") != "spx:iam:ratelimit:ip:digest" {
		t.Fatal("invalid IP key")
	}

	if IAMLoginAccount.Format("digest") != "spx:iam:ratelimit:account:digest" {
		t.Fatal("invalid account key")
	}

	if IAMLoginIP.Format("same") == IAMLoginAccount.Format("same") {
		t.Fatal("rate-limit buckets collided")
	}
}
