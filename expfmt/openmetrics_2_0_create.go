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
	"bufio"
	"fmt"
	"io"
	"math"
	"strconv"

	dto "github.com/prometheus/client_model/go"
)

// MetricFamilyToOpenMetrics20 converts a MetricFamily proto message into the
// OpenMetrics text format version 2.0.0 and writes the resulting lines to 'out'.
// It returns the number of bytes written and any error encountered.
func MetricFamilyToOpenMetrics20(out io.Writer, in *dto.MetricFamily, options ...EncoderOption) (written int, err error) {
	name := in.GetName()
	if name == "" {
		return 0, fmt.Errorf("MetricFamily has no name: %s", in)
	}

	// Try the interface upgrade. If it doesn't work, we'll use a
	// bufio.Writer from the sync.Pool.
	w, ok := out.(enhancedWriter)
	if !ok {
		b := bufPool.Get().(*bufio.Writer)
		b.Reset(out)
		w = b
		defer func() {
			bErr := b.Flush()
			if err == nil {
				err = bErr
			}
			bufPool.Put(b)
		}()
	}

	var (
		n          int
		metricType = in.GetType()
	)

	// Comments, first HELP, then TYPE.
	if in.Help != nil {
		n, err = w.WriteString("# HELP ")
		written += n
		if err != nil {
			return written, err
		}
		n, err = writeName(w, name)
		written += n
		if err != nil {
			return written, err
		}
		err = w.WriteByte(' ')
		written++
		if err != nil {
			return written, err
		}
		n, err = writeEscapedString(w, *in.Help, true)
		written += n
		if err != nil {
			return written, err
		}
		err = w.WriteByte('\n')
		written++
		if err != nil {
			return written, err
		}
	}
	n, err = w.WriteString("# TYPE ")
	written += n
	if err != nil {
		return written, err
	}
	n, err = writeName(w, name)
	written += n
	if err != nil {
		return written, err
	}
	switch metricType {
	case dto.MetricType_COUNTER:
		n, err = w.WriteString(" counter\n")
	case dto.MetricType_GAUGE:
		n, err = w.WriteString(" gauge\n")
	case dto.MetricType_SUMMARY:
		n, err = w.WriteString(" summary\n")
	case dto.MetricType_UNTYPED:
		n, err = w.WriteString(" unknown\n")
	case dto.MetricType_HISTOGRAM:
		n, err = w.WriteString(" histogram\n")
	case dto.MetricType_GAUGE_HISTOGRAM:
		n, err = w.WriteString(" gaugehistogram\n")
	default:
		return written, fmt.Errorf("unknown metric type %s", metricType.String())
	}
	written += n
	if err != nil {
		return written, err
	}
	if in.Unit != nil {
		n, err = w.WriteString("# UNIT ")
		written += n
		if err != nil {
			return written, err
		}
		n, err = writeName(w, name)
		written += n
		if err != nil {
			return written, err
		}

		err = w.WriteByte(' ')
		written++
		if err != nil {
			return written, err
		}
		n, err = writeEscapedString(w, *in.Unit, true)
		written += n
		if err != nil {
			return written, err
		}
		err = w.WriteByte('\n')
		written++
		if err != nil {
			return written, err
		}
	}

	// Finally the samples, one line for each.
	for _, metric := range in.Metric {
		switch metricType {
		case dto.MetricType_COUNTER:
			if metric.Counter == nil {
				return written, fmt.Errorf("expected counter in metric %s %s", name, metric)
			}
			n, err = writeOpenMetrics20Sample(w, name, metric, metric.Counter.GetValue(), 0, false, metric.Counter.Exemplar)
		case dto.MetricType_GAUGE:
			if metric.Gauge == nil {
				return written, fmt.Errorf("expected gauge in metric %s %s", name, metric)
			}
			n, err = writeOpenMetrics20Sample(w, name, metric, metric.Gauge.GetValue(), 0, false, nil)
		case dto.MetricType_UNTYPED:
			if metric.Untyped == nil {
				return written, fmt.Errorf("expected untyped in metric %s %s", name, metric)
			}
			n, err = writeOpenMetrics20Sample(w, name, metric, metric.Untyped.GetValue(), 0, false, nil)
		case dto.MetricType_SUMMARY:
			if metric.Summary == nil {
				return written, fmt.Errorf("expected summary in metric %s %s", name, metric)
			}
			n, err = writeCompositeSummary(w, name, metric)
		case dto.MetricType_HISTOGRAM, dto.MetricType_GAUGE_HISTOGRAM:
			if metric.Histogram == nil {
				return written, fmt.Errorf("expected histogram in metric %s %s", name, metric)
			}
			n, err = writeCompositeHistogram(w, name, metric, metricType == dto.MetricType_GAUGE_HISTOGRAM)
		default:
			return written, fmt.Errorf("unexpected type in metric %s %s", name, metric)
		}
		written += n
		if err != nil {
			return written, err
		}
	}
	return written, nil
}

// writeOpenMetrics20Sample writes a single sample for simple types (Counter, Gauge, Untyped).
func writeOpenMetrics20Sample(w enhancedWriter, name string, metric *dto.Metric, floatValue float64, intValue uint64, useIntValue bool, exemplar *dto.Exemplar) (int, error) {
	written := 0
	n, err := writeOpenMetricsNameAndLabelPairs(w, name, metric.Label, "", 0)
	written += n
	if err != nil {
		return written, err
	}
	err = w.WriteByte(' ')
	written++
	if err != nil {
		return written, err
	}

	if useIntValue {
		n, err = writeUint(w, intValue)
	} else {
		n, err = writeFloat(w, floatValue)
	}
	written += n
	if err != nil {
		return written, err
	}

	if metric.TimestampMs != nil {
		err = w.WriteByte(' ')
		written++
		if err != nil {
			return written, err
		}
		n, err = writeOpenMetrics20Timestamp(w, float64(*metric.TimestampMs)/1000)
		written += n
		if err != nil {
			return written, err
		}
	}

	// Start Timestamp for Counter
	if metric.Counter != nil && metric.Counter.CreatedTimestamp != nil {
		n, err = w.WriteString(" st@")
		written += n
		if err != nil {
			return written, err
		}
		ts := metric.Counter.CreatedTimestamp
		n, err = writeOpenMetrics20Timestamp(w, float64(ts.GetSeconds())+float64(ts.GetNanos())/1e9)
		written += n
		if err != nil {
			return written, err
		}
	}

	if exemplar != nil && len(exemplar.Label) > 0 {
		n, err = writeExemplar(w, exemplar)
		written += n
		if err != nil {
			return written, err
		}
	}

	err = w.WriteByte('\n')
	written++
	if err != nil {
		return written, err
	}
	return written, nil
}

// writeCompositeSummary writes a summary as a composite value.
func writeCompositeSummary(w enhancedWriter, name string, metric *dto.Metric) (int, error) {
	written := 0
	n, err := writeOpenMetricsNameAndLabelPairs(w, name, metric.Label, "", 0)
	written += n
	if err != nil {
		return written, err
	}
	err = w.WriteByte(' ')
	written++
	if err != nil {
		return written, err
	}

	err = w.WriteByte('{')
	written++
	if err != nil {
		return written, err
	}

	// count
	n, err = w.WriteString("count:")
	written += n
	if err != nil {
		return written, err
	}
	n, err = writeUint(w, metric.Summary.GetSampleCount())
	written += n
	if err != nil {
		return written, err
	}

	// sum
	n, err = w.WriteString(",sum:")
	written += n
	if err != nil {
		return written, err
	}
	n, err = writeFloat(w, metric.Summary.GetSampleSum())
	written += n
	if err != nil {
		return written, err
	}

	// quantiles
	n, err = w.WriteString(",quantile:[")
	written += n
	if err != nil {
		return written, err
	}
	for i, q := range metric.Summary.Quantile {
		if i > 0 {
			err = w.WriteByte(',')
			written++
			if err != nil {
				return written, err
			}
		}
		n, err = writeFloat(w, q.GetQuantile())
		written += n
		if err != nil {
			return written, err
		}
		err = w.WriteByte(':')
		written++
		if err != nil {
			return written, err
		}
		n, err = writeFloat(w, q.GetValue())
		written += n
		if err != nil {
			return written, err
		}
	}
	err = w.WriteByte(']')
	written++
	if err != nil {
		return written, err
	}

	err = w.WriteByte('}')
	written++
	if err != nil {
		return written, err
	}

	// Timestamp
	if metric.TimestampMs != nil {
		err = w.WriteByte(' ')
		written++
		if err != nil {
			return written, err
		}
		n, err = writeOpenMetrics20Timestamp(w, float64(*metric.TimestampMs)/1000)
		written += n
		if err != nil {
			return written, err
		}
	}

	// Start Timestamp for Summary
	if metric.Summary.CreatedTimestamp != nil {
		n, err = w.WriteString(" st@")
		written += n
		if err != nil {
			return written, err
		}
		ts := metric.Summary.CreatedTimestamp
		n, err = writeOpenMetrics20Timestamp(w, float64(ts.GetSeconds())+float64(ts.GetNanos())/1e9)
		written += n
		if err != nil {
			return written, err
		}
	}

	err = w.WriteByte('\n')
	written++
	if err != nil {
		return written, err
	}

	return written, nil
}

// writeCompositeHistogram writes a histogram as a composite value.
func writeCompositeHistogram(w enhancedWriter, name string, metric *dto.Metric, isGauge bool) (int, error) {
	written := 0
	n, err := writeOpenMetricsNameAndLabelPairs(w, name, metric.Label, "", 0)
	written += n
	if err != nil {
		return written, err
	}
	err = w.WriteByte(' ')
	written++
	if err != nil {
		return written, err
	}

	err = w.WriteByte('{')
	written++
	if err != nil {
		return written, err
	}

	h := metric.Histogram

	// count / gcount
	if isGauge {
		n, err = w.WriteString("gcount:")
	} else {
		n, err = w.WriteString("count:")
	}
	written += n
	if err != nil {
		return written, err
	}
	if h.SampleCountFloat != nil && *h.SampleCountFloat > 0 {
		n, err = writeFloat(w, *h.SampleCountFloat)
	} else {
		n, err = writeUint(w, h.GetSampleCount())
	}
	written += n
	if err != nil {
		return written, err
	}

	// sum / gsum
	if isGauge {
		n, err = w.WriteString(",gsum:")
	} else {
		n, err = w.WriteString(",sum:")
	}
	written += n
	if err != nil {
		return written, err
	}
	n, err = writeFloat(w, h.GetSampleSum())
	written += n
	if err != nil {
		return written, err
	}

	// Check if it is a native histogram
	isNative := h.Schema != nil

	if isNative {
		n, err = fmt.Fprintf(w, ",schema:%d,zero_threshold:", h.GetSchema())
		written += n
		if err != nil {
			return written, err
		}
		n, err = writeFloat(w, h.GetZeroThreshold())
		written += n
		if err != nil {
			return written, err
		}
		n, err = w.WriteString(",zero_count:")
		written += n
		if err != nil {
			return written, err
		}
		if h.ZeroCountFloat != nil && *h.ZeroCountFloat > 0 {
			n, err = writeFloat(w, *h.ZeroCountFloat)
		} else {
			n, err = writeUint(w, h.GetZeroCount())
		}
		written += n
		if err != nil {
			return written, err
		}

		// Spans and Buckets
		if len(h.NegativeSpan) > 0 {
			n, err = w.WriteString(",negative_spans:[")
			written += n
			if err != nil {
				return written, err
			}
			n, err = writeSpans(w, h.NegativeSpan)
			written += n
			if err != nil {
				return written, err
			}
			err = w.WriteByte(']')
			written++
			if err != nil {
				return written, err
			}

			n, err = w.WriteString(",negative_buckets:[")
			written += n
			if err != nil {
				return written, err
			}
			n, err = writeBuckets(w, h.NegativeDelta, h.NegativeCount)
			written += n
			if err != nil {
				return written, err
			}
			err = w.WriteByte(']')
			written++
			if err != nil {
				return written, err
			}
		}

		if len(h.PositiveSpan) > 0 {
			n, err = w.WriteString(",positive_spans:[")
			written += n
			if err != nil {
				return written, err
			}
			n, err = writeSpans(w, h.PositiveSpan)
			written += n
			if err != nil {
				return written, err
			}
			err = w.WriteByte(']')
			written++
			if err != nil {
				return written, err
			}

			n, err = w.WriteString(",positive_buckets:[")
			written += n
			if err != nil {
				return written, err
			}
			n, err = writeBuckets(w, h.PositiveDelta, h.PositiveCount)
			written += n
			if err != nil {
				return written, err
			}
			err = w.WriteByte(']')
			written++
			if err != nil {
				return written, err
			}
		}
	}

	// Classic buckets (always allowed, even for native histograms)
	if len(h.Bucket) > 0 {
		n, err = w.WriteString(",bucket:[")
		written += n
		if err != nil {
			return written, err
		}
		infSeen := false
		for i, b := range h.Bucket {
			if i > 0 {
				err = w.WriteByte(',')
				written++
				if err != nil {
					return written, err
				}
			}
			if math.IsInf(b.GetUpperBound(), +1) {
				n, err = w.WriteString("+Inf")
				infSeen = true
			} else {
				n, err = writeFloat(w, b.GetUpperBound())
			}
			written += n
			if err != nil {
				return written, err
			}
			err = w.WriteByte(':')
			written++
			if err != nil {
				return written, err
			}
			if b.GetCumulativeCountFloat() > 0 {
				n, err = writeFloat(w, b.GetCumulativeCountFloat())
			} else {
				n, err = writeUint(w, b.GetCumulativeCount())
			}
			written += n
			if err != nil {
				return written, err
			}
		}
		if !infSeen {
			if len(h.Bucket) > 0 {
				err = w.WriteByte(',')
				written++
				if err != nil {
					return written, err
				}
			}
			n, err = w.WriteString("+Inf:")
			written += n
			if err != nil {
				return written, err
			}
			if h.SampleCountFloat != nil && *h.SampleCountFloat > 0 {
				n, err = writeFloat(w, *h.SampleCountFloat)
			} else {
				n, err = writeUint(w, h.GetSampleCount())
			}
			written += n
			if err != nil {
				return written, err
			}
		}
		err = w.WriteByte(']')
		written++
		if err != nil {
			return written, err
		}
	}

	err = w.WriteByte('}')
	written++
	if err != nil {
		return written, err
	}

	// Timestamp
	if metric.TimestampMs != nil {
		err = w.WriteByte(' ')
		written++
		if err != nil {
			return written, err
		}
		n, err = writeOpenMetrics20Timestamp(w, float64(*metric.TimestampMs)/1000)
		written += n
		if err != nil {
			return written, err
		}
	}

	// Start Timestamp for Histogram
	if h.CreatedTimestamp != nil {
		n, err = w.WriteString(" st@")
		written += n
		if err != nil {
			return written, err
		}
		ts := h.CreatedTimestamp
		n, err = writeOpenMetrics20Timestamp(w, float64(ts.GetSeconds())+float64(ts.GetNanos())/1e9)
		written += n
		if err != nil {
			return written, err
		}
	}

	// Exemplars
	// Prefer ones on classic buckets if present, or fall back to the ones on the histogram itself.
	var exemplars []*dto.Exemplar
	for _, b := range h.Bucket {
		if b.Exemplar != nil {
			exemplars = append(exemplars, b.Exemplar)
		}
	}
	if len(exemplars) == 0 {
		exemplars = h.Exemplars
	}

	for _, e := range exemplars {
		n, err = writeExemplar(w, e)
		written += n
		if err != nil {
			return written, err
		}
	}

	err = w.WriteByte('\n')
	written++
	if err != nil {
		return written, err
	}

	return written, nil
}

func writeSpans(w enhancedWriter, spans []*dto.BucketSpan) (int, error) {
	written := 0
	for i, s := range spans {
		if i > 0 {
			err := w.WriteByte(',')
			written++
			if err != nil {
				return written, err
			}
		}
		n, err := fmt.Fprintf(w, "%d:%d", s.GetOffset(), s.GetLength())
		written += n
		if err != nil {
			return written, err
		}
	}
	return written, nil
}

func writeBuckets(w enhancedWriter, deltas []int64, counts []float64) (int, error) {
	written := 0
	if len(counts) > 0 {
		for i, c := range counts {
			if i > 0 {
				err := w.WriteByte(',')
				written++
				if err != nil {
					return written, err
				}
			}
			n, err := writeFloat(w, c)
			written += n
			if err != nil {
				return written, err
			}
		}
	} else if len(deltas) > 0 {
		var current int64
		for i, d := range deltas {
			if i > 0 {
				err := w.WriteByte(',')
				written++
				if err != nil {
					return written, err
				}
			}
			current += d
			n, err := writeUint(w, uint64(current))
			written += n
			if err != nil {
				return written, err
			}
		}
	}
	return written, nil
}

// writeOpenMetrics20Timestamp writes a float64 as a timestamp without scientific notation.
func writeOpenMetrics20Timestamp(w enhancedWriter, f float64) (int, error) {
	switch {
	case math.IsNaN(f):
		return w.WriteString("NaN")
	case math.IsInf(f, +1):
		return w.WriteString("+Inf")
	case math.IsInf(f, -1):
		return w.WriteString("-Inf")
	default:
		bp := numBufPool.Get().(*[]byte)
		*bp = strconv.AppendFloat((*bp)[:0], f, 'f', -1, 64)
		written, err := w.Write(*bp)
		numBufPool.Put(bp)
		return written, err
	}
}
