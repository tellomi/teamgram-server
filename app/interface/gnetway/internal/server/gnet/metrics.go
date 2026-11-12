//  Copyright (c) 2026 The Teamgram Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gnet

import "github.com/zeromicro/go-zero/core/metric"

const namespace = "gnetway"

var (
	metricConnOpen = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: namespace,
		Subsystem: "conn",
		Name:      "open_total",
		Help:      "Total number of connections opened.",
		Labels:    []string{"proto"}, // tcp, websocket
	})

	metricConnClose = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: namespace,
		Subsystem: "conn",
		Name:      "close_total",
		Help:      "Total number of connections closed.",
		Labels:    []string{"proto"},
	})

	metricConnTimeout = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: namespace,
		Subsystem: "conn",
		Name:      "timeout_total",
		Help:      "Total number of connections closed by timeout.",
		Labels:    []string{},
	})

	metricHandshake = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: namespace,
		Subsystem: "handshake",
		Name:      "total",
		Help:      "Total number of handshake attempts.",
		Labels:    []string{"result"}, // ok, error
	})

	metricMsgProcess = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: namespace,
		Subsystem: "msg",
		Name:      "process_duration_ms",
		Help:      "Message processing duration in milliseconds.",
		Labels:    []string{"type"}, // encrypted, unencrypted
		Buckets:   []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 5000},
	})

	metricQuickAck = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: namespace,
		Subsystem: "msg",
		Name:      "quick_ack_total",
		Help:      "Total number of quick ACK responses sent.",
		Labels:    []string{},
	})

	metricCodecDecodeError = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: namespace,
		Subsystem: "codec",
		Name:      "decode_errors_total",
		Help:      "Total number of MTProto codec decode errors, labeled by transport and reason.",
		Labels:    []string{"proto", "reason"}, // proto: tcp, websocket; reason: bad_magic, bad_len, bad_crc, bad_seq, decrypt, transport_unsupported, unexpected_eof, other
	})

	// Entry counters use a fixed class. They do not carry an address,
	// auth key, session id, or user id.
	entryConnMetric = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: namespace,
		Subsystem: "entry",
		Name:      "conn_total",
		Help:      "Accepted native MTProto connections by entry class.",
		Labels:    []string{"class"},
	})
	entryBytesMetric = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: namespace,
		Subsystem: "entry",
		Name:      "bytes_total",
		Help:      "Bytes on an accepted native connection. dir is in or out. The PROXY header is not included.",
		Labels:    []string{"class", "dir"},
	})
	entryRejectMetric = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: namespace,
		Subsystem: "entry",
		Name:      "reject_total",
		Help:      "PROXY listener closes before a connection is counted. reason is a fixed token.",
		Labels:    []string{"reason"},
	})
)
