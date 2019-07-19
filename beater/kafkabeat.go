package beater

import (
	"fmt"
	"time"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"

	"github.com/dmitryilyin/kafkabeat/config"

	"github.com/Shopify/sarama"
)

// Kafkabeat configuration.
type Kafkabeat struct {
	done   chan struct{}

	mode   beat.PublishMode
	sarama *sarama.Config

	config config.Config
	client beat.Client
}

// New creates an instance of kafkabeat.
func New(b *beat.Beat, cfg *common.Config) (beat.Beater, error) {
	c := config.DefaultConfig
	if err := cfg.Unpack(&c); err != nil {
		return nil, fmt.Errorf("Error reading config file: %v", err)
	}

	sarama := sarama.NewConfig()
	sarama.ClientID = c.ClientID
	sarama.ChannelBufferSize = c.ChannelBufferSize
	sarama.Version = parseKafkaVersion(c.Version)



	bt := &Kafkabeat{
		done:   make(chan struct{}),
		config: c,
	}
	return bt, nil
}

// Run starts kafkabeat.
func (bt *Kafkabeat) Run(b *beat.Beat) error {
	logp.Info("kafkabeat is running! Hit CTRL-C to stop it.")

	var err error
	bt.client, err = b.Publisher.Connect()
	if err != nil {
		return err
	}

	ticker := time.NewTicker(1 * time.Second)
	counter := 1
	for {
		select {
		case <-bt.done:
			return nil
		case <-ticker.C:
		}

		event := beat.Event{
			Timestamp: time.Now(),
			Fields: common.MapStr{
				"type":    b.Info.Name,
				"counter": counter,
			},
		}
		bt.client.Publish(event)
		logp.Info("Event sent")
		counter++
	}
}

// Stop stops kafkabeat.
func (bt *Kafkabeat) Stop() {
	_ = bt.client.Close()
	close(bt.done)
}

var kafkaVersions = map[string]sarama.KafkaVersion{
	"":         sarama.V0_10_2_0,
	"0.8.0":    sarama.V0_8_2_0,
	"0.8.1":    sarama.V0_8_2_1,
	"0.8.2":    sarama.V0_8_2_2,
	"0.8":      sarama.V0_8_2_0,
	"0.9.0.0":  sarama.V0_9_0_0,
	"0.9.0.1":  sarama.V0_9_0_1,
	"0.9.0":    sarama.V0_9_0_0,
	"0.9":      sarama.V0_9_0_0,
	"0.10.0.0": sarama.V0_10_0_0,
	"0.10.0.1": sarama.V0_10_0_1,
	"0.10.0":   sarama.V0_10_0_0,
	"0.10.1.0": sarama.V0_10_1_0,
	"0.10.1":   sarama.V0_10_1_0,
	"0.10.2.0": sarama.V0_10_2_0,
	"0.10.2.1": sarama.V0_10_2_0,
	"0.10.2":   sarama.V0_10_2_0,
	"0.10":     sarama.V0_10_0_0,
	"0.11.0.1": sarama.V0_11_0_0,
	"0.11.0.2": sarama.V0_11_0_0,
	"0.11.0":   sarama.V0_11_0_0,
	"1.0.0":    sarama.V1_0_0_0,
	"1.1.0":    sarama.V1_1_0_0,
	"1.1.1":    sarama.V1_1_0_0,
	"2.0.0":    sarama.V2_0_0_0,
	"2.0.1":    sarama.V2_0_0_0,
	"2.1.0":    sarama.V2_1_0_0,
	"2.2.0":    sarama.V2_2_0_0,
}

func parseKafkaVersion(kafkaVersion string) sarama.KafkaVersion {
	version, ok := kafkaVersions[string(kafkaVersion)]
	if !ok {
		panic("Unknown Kafka Version: " + kafkaVersion)
	}

	return version
}
