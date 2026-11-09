package order

import (
	"github.com/fixpro/server/internal/platform/appointment"
	"testing"
	"time"
)

func TestDaytimeAppointmentAndBeijingDate(t *testing.T) {
	for _, slot := range []string{"08:00", "10:00", "12:00", "14:00", "16:00"} {
		if !validSlot(slot) {
			t.Fatal(slot)
		}
	}
	for _, slot := range []string{"18:00", "20:00", "07:00", "16:30", ""} {
		if validSlot(slot) {
			t.Fatal("non-daytime accepted", slot)
		}
	}
	previous := time.Local
	time.Local = time.FixedZone("different host", -12*3600)
	defer func() { time.Local = previous }()
	w := Write{ContactName: "测试客户", ContactMobile: "13800138000", ServiceAddress: "汉川市测试地址", AppointmentSlot: "16:00"}
	today := time.Now().In(appointment.Location)
	for _, days := range []int{1, 30} {
		w.AppointmentDate = today.AddDate(0, 0, days).Format("2006-01-02")
		if e := validate(w); e != nil {
			t.Fatal("valid Beijing date rejected", e)
		}
	}
	for _, days := range []int{0, 31} {
		w.AppointmentDate = today.AddDate(0, 0, days).Format("2006-01-02")
		if e := validate(w); e == nil {
			t.Fatal("invalid date accepted", days)
		}
	}
}
