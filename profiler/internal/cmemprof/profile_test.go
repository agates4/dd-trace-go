package cmemprof_test

import (
	"regexp"
	"runtime"
	"sync"
	"testing"

	"gopkg.in/DataDog/dd-trace-go.v1/profiler/internal/cmemprof"
	"gopkg.in/DataDog/dd-trace-go.v1/profiler/internal/cmemprof/testallocator"
)

func TestCAllocationProfiler(t *testing.T) {
	var prof cmemprof.Profile
	prof.Start(1)

	testallocator.DoAllocC(32)
	testallocator.DoCalloc(32)
	testallocator.DoAllocC(32)
	testallocator.DoCalloc(32)

	pprof, err := prof.Stop()
	if err != nil {
		t.Fatalf("running profile: %s", err)
	}

	err = pprof.CheckValid()
	if err != nil {
		t.Fatalf("checking validity: %s", err)
	}
	original := pprof.Copy()
	found, _, _, _ := pprof.FilterSamplesByName(regexp.MustCompile("[A-a]lloc"), nil, nil, nil)
	if !found {
		t.Logf("%s", original)
		t.Fatal("did not find any allocation samples")
	}
	if len(pprof.Sample) != 4 {
		t.Errorf("got %d samples, wanted 4", len(pprof.Sample))
	}
	for _, sample := range pprof.Sample {
		t.Log("--------")
		for _, loc := range sample.Location {
			t.Logf("%x %s", loc.Address, loc.Line[0].Function.Name)
		}
		if len(sample.Value) != 4 {
			t.Fatalf("sample should have 4 values")
		}
		count := sample.Value[0]
		size := sample.Value[1]
		t.Logf("count=%d, size=%d", count, size)
		if count != 1 {
			t.Errorf("got %d count, wanted 1", count)
		}
		if size != 32 {
			t.Errorf("got %d size, wanted 32", size)
		}
	}
}

// TestCgoMallocNoPanic checks that function which calls C.malloc will not cause
// the profiler to panic (by causing stack growth and invalidating the address
// where the result of C.malloc returns)
func TestCgoMallocNoPanic(t *testing.T) {
	var prof cmemprof.Profile
	prof.Start(1)

	testallocator.DoAllocGo(32)
	testallocator.DoAllocGo(32)
	testallocator.DoAllocGo(32)
	testallocator.DoAllocGo(32)

	_, err := prof.Stop()
	if err != nil {
		t.Fatalf("running profile: %s", err)
	}
}

// TestNewCgoThreadCrash checks that wrapping malloc does not cause creating a
// new Go runtime "m" (OS thread) to crash. For cgo programs, creating a new m
// calls malloc, and the malloc wrapper calls into Go code, which can't be done
// on a new m with no goroutine.
func TestNewCgoThreadCrash(t *testing.T) {
	var prof cmemprof.Profile
	prof.Start(1)

	var ready sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < runtime.GOMAXPROCS(0)*2; i++ {
		ready.Add(1)
		go func() {
			// By locking this OS thread we should force the Go
			// runtime to start making more threads
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			ready.Done()
			<-stop
		}()
	}
	ready.Wait()
	close(stop)

	_, err := prof.Stop()
	if err != nil {
		t.Fatalf("running profile: %s", err)
	}
}