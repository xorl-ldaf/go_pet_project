package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "todo_http_requests_total",
		Help: "Total HTTP requests.",
	}, []string{"method", "route", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "todo_http_request_duration_seconds",
		Help:    "HTTP request duration in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route", "status"})

	remindersProcessedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "todo_reminders_processed_total",
		Help: "Total reminders processed by the reminder scheduler.",
	})
	reminderSchedulerFailuresTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "todo_reminder_scheduler_failures_total",
		Help: "Total reminder scheduler processing failures.",
	})
	reminderSchedulerBatchDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "todo_reminder_scheduler_batch_duration_seconds",
		Help:    "Reminder scheduler batch duration in seconds.",
		Buckets: prometheus.DefBuckets,
	})

	kafkaEventsPublishedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "todo_kafka_events_published_total",
		Help: "Total Kafka events published.",
	})
	kafkaPublishFailuresTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "todo_kafka_publish_failures_total",
		Help: "Total Kafka publish failures.",
	})
	kafkaConsumerProcessedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "todo_kafka_consumer_processed_total",
		Help: "Total Kafka records processed by consumers.",
	})
	kafkaConsumerFailuresTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "todo_kafka_consumer_failures_total",
		Help: "Total Kafka consumer record failures.",
	})

	notificationsCreatedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "todo_notifications_created_total",
		Help: "Total notifications created.",
	})

	telegramDeliverySuccessTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "todo_telegram_delivery_success_total",
		Help: "Total successful Telegram deliveries.",
	})
	telegramDeliveryFailureTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "todo_telegram_delivery_failure_total",
		Help: "Total failed Telegram deliveries.",
	})
	telegramDeliveryRetriesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "todo_telegram_delivery_retries_total",
		Help: "Total Telegram delivery retries scheduled.",
	})

	recurrenceOccurrencesGeneratedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "todo_recurrence_occurrences_generated_total",
		Help: "Total recurrence occurrences generated.",
	})
	recurrenceGenerationFailuresTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "todo_recurrence_generation_failures_total",
		Help: "Total recurrence generation failures.",
	})
)

func Handler() http.Handler {
	return promhttp.Handler()
}

func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		startedAt := time.Now()

		next.ServeHTTP(recorder, r)

		route := r.Pattern
		if route == "" {
			route = "unknown"
		}
		status := strconv.Itoa(recorder.status)
		httpRequestsTotal.WithLabelValues(r.Method, route, status).Inc()
		httpRequestDuration.WithLabelValues(r.Method, route, status).Observe(time.Since(startedAt).Seconds())
	})
}

func ObserveReminderSchedulerBatch(processed int, duration time.Duration, failed bool) {
	reminderSchedulerBatchDuration.Observe(duration.Seconds())
	if processed > 0 {
		remindersProcessedTotal.Add(float64(processed))
	}
	if failed {
		reminderSchedulerFailuresTotal.Inc()
	}
}

func ObserveKafkaPublished() {
	kafkaEventsPublishedTotal.Inc()
}

func ObserveKafkaPublishFailure() {
	kafkaPublishFailuresTotal.Inc()
}

func ObserveKafkaConsumerProcessed() {
	kafkaConsumerProcessedTotal.Inc()
}

func ObserveKafkaConsumerFailure() {
	kafkaConsumerFailuresTotal.Inc()
}

func ObserveNotificationCreated() {
	notificationsCreatedTotal.Inc()
}

func ObserveTelegramDeliverySuccess() {
	telegramDeliverySuccessTotal.Inc()
}

func ObserveTelegramDeliveryFailure() {
	telegramDeliveryFailureTotal.Inc()
}

func ObserveTelegramDeliveryRetry() {
	telegramDeliveryRetriesTotal.Inc()
}

func ObserveRecurrenceGenerated(count int) {
	if count > 0 {
		recurrenceOccurrencesGeneratedTotal.Add(float64(count))
	}
}

func ObserveRecurrenceGenerationFailure() {
	recurrenceGenerationFailuresTotal.Inc()
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
