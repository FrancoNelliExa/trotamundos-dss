package kafka

import (
	"context"
	"encoding/json"
	"time"

	"github.com/FrancoNelliExa/trotamundos-dss/services/itinerary-service/internal/domain/models"
	kafkago "github.com/segmentio/kafka-go"
)

type KafkaPublisher struct {
	writer *kafkago.Writer
}

func NewKafkaPublisher(broker, topic string) *KafkaPublisher {
	return &KafkaPublisher{
		writer: &kafkago.Writer{
			Addr:                   kafkago.TCP(broker),
			Topic:                  topic,
			Balancer:               &kafkago.Hash{},
			AllowAutoTopicCreation: true,
			Async:                  true,
			WriteTimeout:           2 * time.Second,
			BatchTimeout:           20 * time.Millisecond,
		},
	}
}

func (p *KafkaPublisher) PublishItineraryUpdated(ctx context.Context, event models.ItineraryUpdatedEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafkago.Message{
		Key:   []byte(event.UsuarioID),
		Value: payload,
	})
}

func (p *KafkaPublisher) Close() error {
	return p.writer.Close()
}
