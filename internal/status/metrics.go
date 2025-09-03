package status

import "github.com/prometheus/client_golang/prometheus"

var (
	sessionsStarted = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "m17_sessions_started_total",
		Help: "Total number of sessions started.",
	})
	sessionsEnded = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "m17_sessions_ended_total",
		Help: "Total number of sessions ended.",
	})
	pttEvents = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "m17_ptt_events_total",
		Help: "Total number of push-to-talk events.",
	})
	heartbeats = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "m17_heartbeat_total",
		Help: "Total number of heartbeat messages.",
	})
	activeSessions = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "m17_sessions_active",
		Help: "Current number of active sessions.",
	})
	audioFramesDropped = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "m17_audio_frames_dropped_total",
		Help: "Total number of audio frames dropped.",
	})
)

func init() {
	prometheus.MustRegister(sessionsStarted, sessionsEnded, pttEvents, heartbeats, activeSessions, audioFramesDropped)
}

func RecordSessionStarted() {
	sessionsStarted.Inc()
	activeSessions.Inc()
}

func RecordSessionEnded() {
	sessionsEnded.Inc()
	activeSessions.Dec()
}

func RecordPTT() {
	pttEvents.Inc()
}

func RecordHeartbeat(sessionCount int) {
	heartbeats.Inc()
	activeSessions.Set(float64(sessionCount))
}

func RecordAudioFrameDropped() {
	audioFramesDropped.Inc()
}
