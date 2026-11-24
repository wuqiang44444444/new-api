package service

import "strconv"

func positiveBillingMeterCSV(value int64) string {
	if value == 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}
