package kpi

type KpiTime struct {
	startTimeUs uint64 `json:"start_time_us"`
	endTimeUs   uint64 `json:"end_time_us"`
	periodUs    uint64 `json:"period_us"`
}

type Kpi struct {
	time KpiTime `json:"time"`
}
