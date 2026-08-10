package order

import "testing"

func TestValidateContact(t *testing.T) {
	if err := validate(Write{ContactName: "张三", ContactMobile: "13800138000", ServiceAddress: "河南省某县某街道 1 号", AppointmentDate: "2099-01-01", AppointmentSlot: "08:00"}); err != nil {
		t.Fatal(err)
	}
	if err := validate(Write{ContactName: "张三", ContactMobile: "123", ServiceAddress: "河南省某县某街道 1 号", AppointmentDate: "2099-01-01", AppointmentSlot: "08:00"}); err == nil {
		t.Fatal("invalid mobile accepted")
	}
}
