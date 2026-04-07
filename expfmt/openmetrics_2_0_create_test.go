// Copyright 2026 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package expfmt

import (
	"bytes"
	"io"
	"math"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestCreateOpenMetrics20(t *testing.T) {
	scenarios := []struct {
		name string
		in   *dto.MetricFamily
		out  string
	}{
		{
			name: "Counter",
			in: &dto.MetricFamily{
				Name: proto.String("http_requests_total"),
				Help: proto.String("Total number of HTTP requests."),
				Type: dto.MetricType_COUNTER.Enum(),
				Metric: []*dto.Metric{
					{
						Label: []*dto.LabelPair{
							{Name: proto.String("method"), Value: proto.String("GET")},
							{Name: proto.String("code"), Value: proto.String("200")},
						},
						Counter: &dto.Counter{
							Value:            proto.Float64(1027),
							CreatedTimestamp: &timestamppb.Timestamp{Seconds: 1234567890},
						},
					},
				},
			},
			out: `# HELP http_requests_total Total number of HTTP requests.
# TYPE http_requests_total counter
http_requests_total{method="GET",code="200"} 1027 st@1234567890
`,
		},
		{
			name: "Gauge",
			in: &dto.MetricFamily{
				Name: proto.String("node_memory_Active_bytes"),
				Help: proto.String("Active memory in bytes."),
				Type: dto.MetricType_GAUGE.Enum(),
				Metric: []*dto.Metric{
					{
						Gauge: &dto.Gauge{
							Value: proto.Float64(1.2345e+09),
						},
					},
				},
			},
			out: `# HELP node_memory_Active_bytes Active memory in bytes.
# TYPE node_memory_Active_bytes gauge
node_memory_Active_bytes 1.2345e+09
`,
		},
		{
			name: "Summary",
			in: &dto.MetricFamily{
				Name: proto.String("rpc_duration_seconds"),
				Help: proto.String("RPC duration in seconds."),
				Type: dto.MetricType_SUMMARY.Enum(),
				Metric: []*dto.Metric{
					{
						Label: []*dto.LabelPair{
							{Name: proto.String("service"), Value: proto.String("foo")},
						},
						Summary: &dto.Summary{
							SampleCount: proto.Uint64(2693),
							SampleSum:   proto.Float64(1756.0473),
							Quantile: []*dto.Quantile{
								{Quantile: proto.Float64(0.5), Value: proto.Float64(0.05)},
								{Quantile: proto.Float64(0.9), Value: proto.Float64(0.1)},
								{Quantile: proto.Float64(0.99), Value: proto.Float64(0.2)},
							},
							CreatedTimestamp: &timestamppb.Timestamp{Seconds: 1234567890},
						},
					},
				},
			},
			out: `# HELP rpc_duration_seconds RPC duration in seconds.
# TYPE rpc_duration_seconds summary
rpc_duration_seconds{service="foo"} {count:2693,sum:1756.0473,quantile:[0.5:0.05,0.9:0.1,0.99:0.2]} st@1234567890
`,
		},
		{
			name: "Histogram",
			in: &dto.MetricFamily{
				Name: proto.String("http_request_duration_seconds"),
				Help: proto.String("HTTP request duration in seconds."),
				Type: dto.MetricType_HISTOGRAM.Enum(),
				Metric: []*dto.Metric{
					{
						Histogram: &dto.Histogram{
							SampleCount: proto.Uint64(2693),
							SampleSum:   proto.Float64(1756.0473),
							Bucket: []*dto.Bucket{
								{UpperBound: proto.Float64(0.05), CumulativeCount: proto.Uint64(240)},
								{UpperBound: proto.Float64(0.1), CumulativeCount: proto.Uint64(412)},
								{UpperBound: proto.Float64(0.2), CumulativeCount: proto.Uint64(592)},
								{UpperBound: proto.Float64(math.Inf(+1)), CumulativeCount: proto.Uint64(2693)},
							},
							CreatedTimestamp: &timestamppb.Timestamp{Seconds: 1234567890},
						},
					},
				},
			},
			out: `# HELP http_request_duration_seconds HTTP request duration in seconds.
# TYPE http_request_duration_seconds histogram
http_request_duration_seconds {count:2693,sum:1756.0473,bucket:[0.05:240,0.1:412,0.2:592,+Inf:2693]} st@1234567890
`,
		},
		{
			name: "Native Histogram",
			in: &dto.MetricFamily{
				Name: proto.String("native_histogram_test"),
				Help: proto.String("A test native histogram."),
				Type: dto.MetricType_HISTOGRAM.Enum(),
				Metric: []*dto.Metric{
					{
						Histogram: &dto.Histogram{
							SampleCount:   proto.Uint64(10),
							SampleSum:     proto.Float64(15.5),
							Schema:        proto.Int32(1),
							ZeroThreshold: proto.Float64(0.001),
							ZeroCount:     proto.Uint64(2),
							PositiveSpan: []*dto.BucketSpan{
								{Offset: proto.Int32(1), Length: proto.Uint32(2)},
							},
							PositiveCount: []float64{1, 2},
						},
					},
				},
			},
			out: `# HELP native_histogram_test A test native histogram.
# TYPE native_histogram_test histogram
native_histogram_test {count:10,sum:15.5,schema:1,zero_threshold:0.001,zero_count:2,positive_spans:[1:2],positive_buckets:[1,2]}
`,
		},
		{
			name: "Native Histogram with Deltas",
			in: &dto.MetricFamily{
				Name: proto.String("native_histogram_deltas"),
				Help: proto.String("A test native histogram with deltas."),
				Type: dto.MetricType_HISTOGRAM.Enum(),
				Metric: []*dto.Metric{
					{
						Histogram: &dto.Histogram{
							SampleCount:   proto.Uint64(10),
							SampleSum:     proto.Float64(15.5),
							Schema:        proto.Int32(1),
							ZeroThreshold: proto.Float64(0.001),
							ZeroCount:     proto.Uint64(2),
							NegativeSpan: []*dto.BucketSpan{
								{Offset: proto.Int32(1), Length: proto.Uint32(2)},
							},
							NegativeDelta: []int64{1, 1},
						},
					},
				},
			},
			out: `# HELP native_histogram_deltas A test native histogram with deltas.
# TYPE native_histogram_deltas histogram
native_histogram_deltas {count:10,sum:15.5,schema:1,zero_threshold:0.001,zero_count:2,negative_spans:[1:2],negative_buckets:[1,2]}
`,
		},
		{
			name: "Counter with Exemplar",
			in: &dto.MetricFamily{
				Name: proto.String("http_requests_exemplar"),
				Help: proto.String("Total number of HTTP requests with exemplar."),
				Type: dto.MetricType_COUNTER.Enum(),
				Metric: []*dto.Metric{
					{
						Counter: &dto.Counter{
							Value: proto.Float64(1027),
							Exemplar: &dto.Exemplar{
								Value: proto.Float64(0.01),
								Label: []*dto.LabelPair{
									{Name: proto.String("trace_id"), Value: proto.String("123")},
								},
							},
						},
					},
				},
			},
			out: `# HELP http_requests_exemplar Total number of HTTP requests with exemplar.
# TYPE http_requests_exemplar counter
http_requests_exemplar 1027 # {trace_id="123"} 0.01
`,
		},
		{
			name: "GaugeHistogram",
			in: &dto.MetricFamily{
				Name: proto.String("gauge_histogram_test"),
				Help: proto.String("A test gauge histogram."),
				Type: dto.MetricType_GAUGE_HISTOGRAM.Enum(),
				Metric: []*dto.Metric{
					{
						Histogram: &dto.Histogram{
							SampleCountFloat: proto.Float64(10.5),
							SampleSum:        proto.Float64(15.5),
							Bucket: []*dto.Bucket{
								{UpperBound: proto.Float64(0.05), CumulativeCountFloat: proto.Float64(2.5)},
								{UpperBound: proto.Float64(math.Inf(+1)), CumulativeCountFloat: proto.Float64(10.5)},
							},
						},
					},
				},
			},
			out: `# HELP gauge_histogram_test A test gauge histogram.
# TYPE gauge_histogram_test gaugehistogram
gauge_histogram_test {gcount:10.5,gsum:15.5,bucket:[0.05:2.5,+Inf:10.5]}
`,
		},
		{
			name: "Native Histogram with Float Counts",
			in: &dto.MetricFamily{
				Name: proto.String("native_histogram_float"),
				Help: proto.String("A test native histogram with float counts."),
				Type: dto.MetricType_HISTOGRAM.Enum(),
				Metric: []*dto.Metric{
					{
						Histogram: &dto.Histogram{
							SampleCountFloat: proto.Float64(10.5),
							SampleSum:        proto.Float64(15.5),
							Schema:           proto.Int32(1),
							ZeroThreshold:    proto.Float64(0.001),
							ZeroCountFloat:   proto.Float64(2.5),
							PositiveSpan: []*dto.BucketSpan{
								{Offset: proto.Int32(1), Length: proto.Uint32(1)},
							},
							PositiveCount: []float64{1.5},
						},
					},
				},
			},
			out: `# HELP native_histogram_float A test native histogram with float counts.
# TYPE native_histogram_float histogram
native_histogram_float {count:10.5,sum:15.5,schema:1,zero_threshold:0.001,zero_count:2.5,positive_spans:[1:1],positive_buckets:[1.5]}
`,
		},
		{
			name: "Native Histogram with Multiple Spans",
			in: &dto.MetricFamily{
				Name: proto.String("native_histogram_multi_span"),
				Help: proto.String("A test native histogram with multiple spans."),
				Type: dto.MetricType_HISTOGRAM.Enum(),
				Metric: []*dto.Metric{
					{
						Histogram: &dto.Histogram{
							SampleCount:   proto.Uint64(10),
							SampleSum:     proto.Float64(15.5),
							Schema:        proto.Int32(1),
							ZeroThreshold: proto.Float64(0.001),
							ZeroCount:     proto.Uint64(2),
							PositiveSpan: []*dto.BucketSpan{
								{Offset: proto.Int32(1), Length: proto.Uint32(1)},
								{Offset: proto.Int32(3), Length: proto.Uint32(1)},
							},
							PositiveCount: []float64{1, 2},
						},
					},
				},
			},
			out: `# HELP native_histogram_multi_span A test native histogram with multiple spans.
# TYPE native_histogram_multi_span histogram
native_histogram_multi_span {count:10,sum:15.5,schema:1,zero_threshold:0.001,zero_count:2,positive_spans:[1:1,3:1],positive_buckets:[1,2]}
`,
		},
		{
			name: "Histogram without Inf bucket",
			in: &dto.MetricFamily{
				Name: proto.String("histogram_no_inf"),
				Help: proto.String("A test histogram without Inf bucket."),
				Type: dto.MetricType_HISTOGRAM.Enum(),
				Metric: []*dto.Metric{
					{
						Histogram: &dto.Histogram{
							SampleCount: proto.Uint64(10),
							SampleSum:   proto.Float64(15.5),
							Bucket: []*dto.Bucket{
								{UpperBound: proto.Float64(0.05), CumulativeCount: proto.Uint64(2)},
							},
						},
					},
				},
			},
			out: `# HELP histogram_no_inf A test histogram without Inf bucket.
# TYPE histogram_no_inf histogram
histogram_no_inf {count:10,sum:15.5,bucket:[0.05:2,+Inf:10]}
`,
		},
		{
			name: "Histogram with Classic and Native Buckets and Exemplars",
			in: &dto.MetricFamily{
				Name: proto.String("histogram_classic_and_native_exemplar"),
				Help: proto.String("A test histogram with classic and native buckets and exemplars."),
				Type: dto.MetricType_HISTOGRAM.Enum(),
				Metric: []*dto.Metric{
					{
						Histogram: &dto.Histogram{
							SampleCount:   proto.Uint64(10),
							SampleSum:     proto.Float64(15.5),
							Schema:        proto.Int32(1),
							ZeroThreshold: proto.Float64(0.001),
							ZeroCount:     proto.Uint64(2),
							PositiveSpan: []*dto.BucketSpan{
								{Offset: proto.Int32(1), Length: proto.Uint32(1)},
							},
							PositiveCount: []float64{1},
							Bucket: []*dto.Bucket{
								{
									UpperBound:      proto.Float64(0.05),
									CumulativeCount: proto.Uint64(2),
									Exemplar: &dto.Exemplar{
										Value: proto.Float64(0.01),
										Label: []*dto.LabelPair{{Name: proto.String("bucket_trace"), Value: proto.String("abc")}},
									},
								},
								{UpperBound: proto.Float64(math.Inf(+1)), CumulativeCount: proto.Uint64(10)},
							},
							Exemplars: []*dto.Exemplar{
								{
									Value: proto.Float64(0.02),
									Label: []*dto.LabelPair{{Name: proto.String("hist_trace"), Value: proto.String("xyz")}},
								},
							},
						},
					},
				},
			},
			out: `# HELP histogram_classic_and_native_exemplar A test histogram with classic and native buckets and exemplars.
# TYPE histogram_classic_and_native_exemplar histogram
histogram_classic_and_native_exemplar {count:10,sum:15.5,schema:1,zero_threshold:0.001,zero_count:2,positive_spans:[1:1],positive_buckets:[1],bucket:[0.05:2,+Inf:10]} # {bucket_trace="abc"} 0.01
`,
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			var buf bytes.Buffer
			_, err := MetricFamilyToOpenMetrics20(&buf, scenario.in)
			if err != nil {
				t.Fatal(err)
			}
			if buf.String() != scenario.out {
				t.Errorf("expected out:\n%s\ngot:\n%s", scenario.out, buf.String())
			}
		})
	}
}

func TestOpenMetrics20UTF8(t *testing.T) {
	mfs := []*dto.MetricFamily{
		{
			Name: proto.String("process.cpu.seconds"),
			Help: proto.String("Total user and system CPU time spent in seconds."),
			Type: dto.MetricType_COUNTER.Enum(),
			Unit: proto.String("seconds"),
			Metric: []*dto.Metric{
				{
					Label: []*dto.LabelPair{
						{Name: proto.String("node.name"), Value: proto.String("my_node")},
					},
					Counter: &dto.Counter{
						Value: proto.Float64(4.20072246e+06),
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	_, err := MetricFamilyToOpenMetrics20(&buf, mfs[0])
	if err != nil {
		t.Fatal(err)
	}

	expected := `# HELP "process.cpu.seconds" Total user and system CPU time spent in seconds.
# TYPE "process.cpu.seconds" counter
# UNIT "process.cpu.seconds" seconds
{"process.cpu.seconds","node.name"="my_node"} 4.20072246e+06
`

	if buf.String() != expected {
		t.Errorf("expected out:\n%s\ngot:\n%s", expected, buf.String())
	}
}
func TestWriteOpenMetrics20Sample_UseIntValue(t *testing.T) {
	var buf bytes.Buffer
	w := enhancedWriter(&buf)

	metric := &dto.Metric{}
	_, err := writeOpenMetrics20Sample(w, "test_int", metric, 0, 123, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	expected := "test_int 123\n"
	if buf.String() != expected {
		t.Errorf("expected %q, got %q", expected, buf.String())
	}
}

type plainWriter struct {
	io.Writer
}

func TestMetricFamilyToOpenMetrics20_BufioFallback(t *testing.T) {
	var buf bytes.Buffer
	pw := &plainWriter{Writer: &buf}

	mf := &dto.MetricFamily{
		Name: proto.String("test_fallback"),
		Type: dto.MetricType_GAUGE.Enum(),
		Metric: []*dto.Metric{
			{
				Gauge: &dto.Gauge{Value: proto.Float64(42)},
			},
		},
	}

	_, err := MetricFamilyToOpenMetrics20(pw, mf)
	if err != nil {
		t.Fatal(err)
	}

	expected := "# TYPE test_fallback gauge\ntest_fallback 42\n"
	if buf.String() != expected {
		t.Errorf("expected %q, got %q", expected, buf.String())
	}
}
func TestWriteOpenMetrics20Timestamp_SpecialValues(t *testing.T) {
	tests := []struct {
		name string
		val  float64
		out  string
	}{
		{"NaN", math.NaN(), "NaN"},
		{"+Inf", math.Inf(+1), "+Inf"},
		{"-Inf", math.Inf(-1), "-Inf"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := enhancedWriter(&buf)
			_, err := writeOpenMetrics20Timestamp(w, tc.val)
			if err != nil {
				t.Fatal(err)
			}
			if buf.String() != tc.out {
				t.Errorf("expected %q, got %q", tc.out, buf.String())
			}
		})
	}
}
