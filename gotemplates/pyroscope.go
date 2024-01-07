package gotemplates

import (
	"runtime"

	"github.com/grafana/pyroscope-go"
)

func initProfile() {
	address := GetEnv("PYROSCOPE_SERVER_ADDRESS", "")
	if address == "" {
		return
	}
	mutexProfileRate := 1
	runtime.SetMutexProfileFraction(mutexProfileRate) // ・・・(1)
	blockProfileRate := 1
	runtime.SetBlockProfileRate(blockProfileRate) // ・・・(2)

	pyroscope.Start(pyroscope.Config{
		ApplicationName: "calculator",
		ServerAddress:   GetEnv("PYROSCOPE_SERVER_ADDRESS", "http://monitoring:4040"),
		Logger:          pyroscope.StandardLogger,
		Tags: map[string]string{
			"hostname": "isuports",
			"version":  AppVersion,
		},

		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,
			pyroscope.ProfileGoroutines,
			pyroscope.ProfileMutexCount,
			pyroscope.ProfileMutexDuration,
			pyroscope.ProfileBlockCount,
			pyroscope.ProfileBlockDuration,
		},
	})
}
