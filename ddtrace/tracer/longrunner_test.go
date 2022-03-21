// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

package tracer

import (
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/stretchr/testify/assert"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/internal"
)

func TestLimitHeartbeat(t *testing.T) {
	tooSmall := int64(10)
	tooBig := 8 * time.Minute.Nanoseconds()
	justRight := 4 * time.Minute.Nanoseconds()

	assert.Equal(t, 20*time.Second, limitHeartbeat(tooSmall))
	assert.Equal(t, (7*time.Minute)+(30*time.Second), limitHeartbeat(tooBig))
	assert.Equal(t, time.Duration(justRight), limitHeartbeat(justRight))
}

func TestLongrunner(t *testing.T) {
	t.Run("WorkRemovesFinishedSpans", func(t *testing.T) {
		lr := longrunner{
			mu: sync.Mutex{},
			spans: map[*span]int{
				&span{
					RWMutex:  sync.RWMutex{},
					Start:    1,
					finished: true,
				}: 1,
			},
		}

		lr.work(1)

		assert.Empty(t, lr.spans)
	})

	t.Run("TrackSpanNoOverwrite", func(t *testing.T) {
		s := &span{}
		lr := longrunner{
			mu: sync.Mutex{},
			spans: map[*span]int{
				s: 3,
			},
		}

		lr.trackSpan(s)

		assert.Equal(t, 3, lr.spans[s])
	})

	t.Run("Work", func(t *testing.T) {
		finishedS := &span{
			RWMutex:  sync.RWMutex{},
			Start:    1,
			finished: true,
		}
		s := &span{
			RWMutex: sync.RWMutex{},
			Start:   1,
		}
		s.context = newSpanContext(s, nil)
		s.context.trace.push(finishedS)
		s.context.trace.finishedOne(finishedS)
		ts := testStatsdClient{}
		lr := longrunner{
			heartbeatInterval: 1,
			statsd:            &ts,
			mu:                sync.Mutex{},
			spans: map[*span]int{
				s: 1,
			},
		}

		lr.work(3)

		assert.NotEmpty(t, lr.spans)
		assert.Equal(t, lr.spans[s], 2)
		assert.Equal(t, s.context.trace.finished, 0, "Already finished span should have been removed")
		assert.Len(t, s.context.trace.spans, 1)
		stats := ts.IncrCalls()
		assert.Len(t, stats, 1)
		assert.Equal(t, "datadog.tracer.longrunning.flushed", stats[0].name)
	})

	t.Run("WorkTooOld", func(t *testing.T) {
		start := time.Unix(0, 0)
		s := &span{
			SpanID:  555,
			RWMutex: sync.RWMutex{},
			Start:   start.UnixNano(),
		}
		s.context = newSpanContext(s, nil)
		ts := testStatsdClient{}
		lr := longrunner{
			heartbeatInterval: 1,
			statsd:            &ts,
			mu:                sync.Mutex{},
			spans: map[*span]int{
				s: 1,
			},
		}

		lr.work(start.Add(13 * time.Hour).UnixNano())

		assert.Empty(t, lr.spans)
		stats := ts.IncrCalls()
		assert.Len(t, stats, 1)
		assert.Equal(t, "datadog.tracer.longrunning.expired", stats[0].name)
	})

	t.Run("StopMultipleCallsOk", func(t *testing.T) {
		lr := startLongrunner(500, &statsd.NoOpClient{})
		lr.stop()
		lr.stop()
	})

	t.Run("longrunningSpansEnabledFalseWithoutInfo", func(t *testing.T) {
		c := config{
			agent: agentFeatures{
				Info: false,
			},
			longRunningEnabled: true,
		}
		assert.False(t, longrunningSpansEnabled(&c))
	})
}

func BenchmarkLR(b *testing.B) {
	internal.SetGlobalTracer(&internal.NoopTracer{})
	lr := startLongrunner(int64(10*time.Millisecond), &statsd.NoOpClient{})
	wg := sync.WaitGroup{}
	for i := 0; i < b.N; i++ {
		for i := 0; i < 100; i++ {
			wg.Add(1)
			//waitTime := i
			go func() {
				s := &span{
					Name:     "testspan",
					Service:  "bench",
					Resource: "",
					SpanID:   random.Uint64(),
					TraceID:  random.Uint64(),
					Start:    now(),
				}
				s.context = newSpanContext(s, nil)
				lr.trackSpan(s)
				//every 10th is "long running"
				//if waitTime%10 == 0 {
				//	time.Sleep(heartbeatInterval * 2)
				//} else {
				//	time.Sleep(time.Duration(waitTime))
				//}
				lr.stopTracking(s)
				wg.Done()
			}()
		}
		wg.Wait()
	}
}

func BenchmarkLRWork(b *testing.B) {
	internal.SetGlobalTracer(&internal.NoopTracer{})
	hb := 1 * time.Hour //Large time so we can call work manually
	lr := startLongrunner(int64(hb), &statsd.NoOpClient{})
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		lr.spans = map[*span]int{}
		var spans []*span
		for i := 0; i < 100; i++ {
			s := &span{
				Name:     "testspan",
				Service:  "bench",
				Resource: "",
				SpanID:   random.Uint64(),
				TraceID:  random.Uint64(),
				Start:    now() - (hb.Nanoseconds() * 2),
			}
			s.context = newSpanContext(s, nil)
			lr.trackSpan(s)
			spans = append(spans, s)
		}
		b.StartTimer()
		lr.work(now())
	}
}

func BenchmarkTracking(b *testing.B) {
	internal.SetGlobalTracer(&internal.NoopTracer{})
	hb := 1 * time.Hour //Large time so we can ignore "work" loop
	lr := startLongrunner(int64(hb), &statsd.NoOpClient{})
	n := now()
	sid := uint64(500)
	tid := uint64(900)
	for i := 0; i < b.N; i++ {
		s := &span{
			Name:     "testspan",
			Service:  "bench",
			Resource: "",
			SpanID:   sid,
			TraceID:  tid,
			Start:    n,
		}
		s.context = newSpanContext(s, nil)
		lr.trackSpan(s)
	}
}

func BenchmarkNoTracking(b *testing.B) {
	internal.SetGlobalTracer(&internal.NoopTracer{})
	n := now()
	sid := uint64(500)
	tid := uint64(900)
	for i := 0; i < b.N; i++ {
		s := &span{
			Name:     "testspan",
			Service:  "bench",
			Resource: "",
			SpanID:   sid,
			TraceID:  tid,
			Start:    n,
		}
		s.context = newSpanContext(s, nil)
	}
}
