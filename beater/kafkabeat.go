package beater

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/dmitryilyin/kafkabeat/config"

	"github.com/Shopify/sarama"
	"github.com/bsm/sarama-cluster"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
)

type msgDecodeFn func(msg *sarama.ConsumerMessage) *beat.Event

type Kafkabeat struct {
	done   chan struct{}
	logger *logp.Logger
	mode   beat.PublishMode

	bConfig config.Config   // beats config
	kConfig *cluster.Config // kafka config

	pipeline beat.Client
	consumer *cluster.Consumer

	codec msgDecodeFn
}

// Creates beater
func New(b *beat.Beat, cfg *common.Config) (beat.Beater, error) {
	bConfig := config.DefaultConfig
	if err := cfg.Unpack(&bConfig); err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
	}

	kConfig := cluster.NewConfig()
	kConfig.ClientID = bConfig.ClientID
	kConfig.ChannelBufferSize = bConfig.ChannelBufferSize
	kConfig.Consumer.MaxWaitTime = time.Millisecond * 500
	kConfig.Consumer.Return.Errors = true

	// initial offset handling
	switch bConfig.Offset {
	case "newest":
		kConfig.Consumer.Offsets.Initial = sarama.OffsetNewest
	case "oldest":
		kConfig.Consumer.Offsets.Initial = sarama.OffsetOldest
	default:
		return nil, fmt.Errorf("error in configuration, unknown offset: '%s'", bConfig.Offset)
	}

	// codec to use
	var codecFn msgDecodeFn
	switch bConfig.Codec {
	case "json":
		codecFn = decodeJson
	case "plain":
		codecFn = decodePlain
	default:
		return nil, fmt.Errorf("error in configuration, unknown codec: '%s'", bConfig.Codec)
	}

	// publish_mode
	var mode beat.PublishMode
	switch bConfig.PublishMode {
	case "default":
		mode = beat.DefaultGuarantees
	case "send":
		mode = beat.GuaranteedSend
	case "drop_if_full":
		mode = beat.DropIfFull
	default:
		return nil, fmt.Errorf("error in configuration, unknown publish_mode: '%s'", bConfig.PublishMode)
	}

	if bConfig.ChannelWorkers < 1 {
		bConfig.ChannelWorkers = 1
	}

	// return beat
	bt := &Kafkabeat{
		done:    make(chan struct{}),
		logger:  logp.NewLogger("kafkabeat"),
		mode:    mode,
		bConfig: bConfig,
		kConfig: kConfig,
		codec:   codecFn,
	}
	return bt, nil
}

func (bt *Kafkabeat) Run(b *beat.Beat) error {
	var err error

	// start kafka consumer
	bt.consumer, err = cluster.NewConsumer(
		bt.bConfig.Brokers,
		bt.bConfig.Group,
		bt.bConfig.Topics,
		bt.kConfig,
	)
	if err != nil {
		return err
	}

	// start beats pipeline
	bt.pipeline, err = b.Publisher.ConnectWith(
		beat.ClientConfig{
			PublishMode: bt.mode,
		},
	)
	if err != nil {
		return err
	}

	// run workers
	bt.logger.Info("spawning channel workers: ", bt.bConfig.ChannelWorkers)
	for i := 0; i < bt.bConfig.ChannelWorkers; i++ {
		go bt.workerFn()
	}

	// run loop
	bt.logger.Info("kafkabeat is running! Hit CTRL-C to stop it.")
	for {
		select {
		case <-bt.done:
			bt.consumer.Close()
			return nil

		case err := <-bt.consumer.Errors():
			bt.logger.Error(err.Error())
		}
	}
}

func (bt *Kafkabeat) workerFn() {
	for {
		msg := <-bt.consumer.Messages()
		if msg == nil {
			break
		}

		if event := bt.codec(msg); event != nil {
			bt.pipeline.Publish(*event)
		}
		bt.consumer.MarkOffset(msg, "")
	}
}

func (bt *Kafkabeat) Stop() {
	bt.pipeline.Close()
	close(bt.done)
}

func decodeJson(msg *sarama.ConsumerMessage) *beat.Event {
	fields := map[string]interface{}{}
	if err := json.Unmarshal(msg.Value, &fields); err != nil {
		return nil
	}

	// special @timestamp field handling
	var ts time.Time
	if val, exists := fields["@timestamp"]; exists {
		delete(fields, "@timestamp") // drop timestamp key

		if s, ok := val.(string); ok {
			if p, err := common.ParseTime(s); err == nil {
				ts = time.Time(p)
			}
		}
	}

	if ts.IsZero() {
		if msg.Timestamp.IsZero() {
			ts = time.Now()
		} else {
			ts = msg.Timestamp
		}
	}

	return &beat.Event{
		Timestamp: ts,
		Fields:    fields,
	}
}

func decodePlain(msg *sarama.ConsumerMessage) *beat.Event {
	fields := map[string]interface{}{
		"message": string(msg.Value),
	}

	ts := msg.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	return &beat.Event{
		Timestamp: ts,
		Fields:    fields,
	}
}
