package order

import "testing"

func TestValidateContact(t *testing.T) {
	if err := validate(Write{"张三", "13800138000", "河南省某县某街道 1 号"}); err != nil {
		t.Fatal(err)
	}
	if err := validate(Write{"张三", "123", "河南省某县某街道 1 号"}); err == nil {
		t.Fatal("invalid mobile accepted")
	}
}
