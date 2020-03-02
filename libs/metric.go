package tshttp

// Metric structure
type Metric struct {
	Name         string  `json:"name"`
	WorkerCount  int     `json:"workerCount"`
	ErrorCount   int     `json:"errorCount"`
	Avg          float64 `json:"avg"`
	Min          float64 `json:"min"`
	Max          float64 `json:"max"`
	ElapedTime   float64 `json:"elapedTime"`
	RequestCount int     `json:"requestCount"`
	Rps          float64 `json:"rps"`
}
