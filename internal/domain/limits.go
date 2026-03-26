package domain

import "time"

type LimitType string

const (
	LimitTypeRPM          LimitType = "rpm"
	LimitTypeDaily        LimitType = "daily"
	LimitTypeWindow5h     LimitType = "window_5h"
	LimitTypeMonthly      LimitType = "monthly"
	LimitTypeTokenDaily   LimitType = "token_daily"
	LimitTypeTokenMonthly LimitType = "token_monthly"
)

type AccountLimits struct {
	RPM          *int
	Daily        *int
	Window5h     *int
	Monthly      *int
	TokenDaily   *int
	TokenMonthly *int
}

type LimitState struct {
	Type        LimitType
	Current     int
	Max         int
	WindowStart time.Time
	WindowEnd   time.Time
}

func (s *LimitState) IsExceeded() bool {
	return s.Current >= s.Max
}

func (s *LimitState) Remaining() int {
	remaining := s.Max - s.Current
	if remaining < 0 {
		return 0
	}
	return remaining
}
