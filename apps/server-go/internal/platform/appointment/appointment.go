// Package appointment defines the supported daytime service windows.
package appointment

import "time"

var Location = time.FixedZone("Asia/Shanghai", 8*60*60)

func ValidSlot(slot string) bool {
	switch slot {
	case "08:00", "10:00", "12:00", "14:00", "16:00":
		return true
	}
	return false
}
