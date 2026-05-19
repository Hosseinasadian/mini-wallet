package rabbitmq

import (
	amqp "github.com/rabbitmq/amqp091-go"
)

type queueNames struct {
	mainExchange string
	dlxExchange  string
	mainQueue    string
	retryQueue   string
	dlq          string
}

func buildNames(eventName, queueBase string) queueNames {
	return queueNames{
		mainExchange: eventName + ".exchange",
		dlxExchange:  eventName + ".dlx",
		mainQueue:    queueBase + ".queue",
		retryQueue:   queueBase + ".retry.queue",
		dlq:          queueBase + ".dlq",
	}
}

func extractRejectedCount(headers amqp.Table, queueName string) int64 {
	if headers == nil {
		return 0
	}

	xDeath, ok := headers["x-death"]
	if !ok {
		return 0
	}

	items, ok := xDeath.([]interface{})
	if !ok {
		return 0
	}

	for _, item := range items {
		entry, ok := item.(amqp.Table)
		if !ok {
			continue
		}

		if queue, ok := entry["queue"].(string); !ok || queue != queueName {
			continue
		}

		if reason, ok := entry["reason"].(string); !ok || reason != "rejected" {
			continue
		}

		count, ok := entry["count"]
		if !ok {
			return 0
		}

		switch v := count.(type) {
		case int64:
			return v
		case int32:
			return int64(v)
		default:
			return 0
		}
	}

	return 0
}
