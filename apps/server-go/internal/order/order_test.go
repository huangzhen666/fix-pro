package order

import (
	"testing"
	"time"
)

func TestValidateContact(t *testing.T) {
	tomorrow := time.Now().In(time.Local).AddDate(0, 0, 1).Format("2006-01-02")
	if err := validate(Write{ContactName: "张三", ContactMobile: "13800138000", ServiceAddress: "河南省某县某街道 1 号", AppointmentDate: tomorrow, AppointmentSlot: "08:00"}); err != nil {
		t.Fatal(err)
	}
	if err := validate(Write{ContactName: "张三", ContactMobile: "123", ServiceAddress: "河南省某县某街道 1 号", AppointmentDate: tomorrow, AppointmentSlot: "08:00"}); err == nil {
		t.Fatal("invalid mobile accepted")
	}
}

func TestValidateAppointmentDateWindow(t *testing.T) {
	now := time.Now().In(time.Local)
	valid := func(days int) string { return now.AddDate(0, 0, days).Format("2006-01-02") }
	base := func(date string) Write {
		return Write{ContactName: "张三", ContactMobile: "13800138000", ServiceAddress: "河南省某县某街道 1 号", AppointmentDate: date, AppointmentSlot: "08:00"}
	}
	if err := validate(base(valid(0))); err == nil {
		t.Fatal("today should not be accepted")
	}
	if err := validate(base(valid(1))); err != nil {
		t.Fatalf("tomorrow should be accepted: %v", err)
	}
	if err := validate(base(valid(30))); err != nil {
		t.Fatalf("30th day should be accepted: %v", err)
	}
	if err := validate(base(valid(31))); err == nil {
		t.Fatal("31st day should not be accepted")
	}
}
