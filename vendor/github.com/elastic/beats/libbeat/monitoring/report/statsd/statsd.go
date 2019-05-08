// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package statsd

import (
	"sync"
	"time"
	gostatsd "github.com/smira/go-statsd"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/monitoring"
	"github.com/elastic/beats/libbeat/monitoring/report"
)

type reporter struct {
	wg       sync.WaitGroup
	done     chan struct{}
	period   time.Duration
	registry *monitoring.Registry
	client   gostatsd.Client
	logger   *logp.Logger
}

// MakeReporter returns a new StatsdReporter that periodically reports metrics via
// statsd protocol over UDP. 
func MakeReporter(beat beat.Info, cfg *common.Config) (report.Reporter, error) {
	config := defaultConfig
	if cfg != nil {
		if err := cfg.Unpack(&config); err != nil {
			return nil, err
		}
	}

	r := &reporter{
		done:     make(chan struct{}),
		period:   config.Period,
		logger:   logp.NewLogger("monitoring"),
		client:   *gostatsd.NewClient(
			config.Address,
			gostatsd.MetricPrefix(config.Prefix),
			gostatsd.TagStyle(gostatsd.TagFormatDatadog),
			gostatsd.DefaultTags(gostatsd.StringTag("host", beat.Hostname)),
			),
		registry: monitoring.Default,
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.snapshotLoop()
	}()
	return r, nil
}

func (r *reporter) Stop() {
	close(r.done)
	r.wg.Wait()
}

func (r *reporter) snapshotLoop() {
	r.logger.Infof("Starting statsd metrics every %v", r.period)
	defer r.logger.Infof("Stopping statsd metrics.")

	ticker := time.NewTicker(r.period)
	defer ticker.Stop()

	for {
		select {
		case <-r.done:
			return
		case <-ticker.C:
		}

		snapshot := makeSnapshot(r.registry)
		r.sendSnapshot(snapshot)
	}
}

func (r *reporter) sendSnapshot(s monitoring.FlatSnapshot) {
	for intMetric, intValue := range s.Ints {
		r.client.Gauge(intMetric, intValue)
	}
	for floatMetric, floatValue := range s.Floats {
		r.client.FGauge(floatMetric, floatValue)
	}	
}

func makeSnapshot(R *monitoring.Registry) monitoring.FlatSnapshot {
	mode := monitoring.Full
	return monitoring.CollectFlatSnapshot(R, mode, true)
}
