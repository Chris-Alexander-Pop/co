package progress

import "time"

const rateWindow = 30 * time.Second

// How long the current run has to be going before its own average replaces
// the last run's speed. Earlier than that, the average is mostly noise.
const trustAfter = 8 * time.Second

type sample struct {
	t        time.Time
	compiled int
}

// Prior is a finished build of the same project: how many compile lines it
// produced, and how long that took.
type Prior struct {
	Files    int
	Duration time.Duration
}

func (p Prior) speed() float64 {
	if p.Files <= 0 || p.Duration <= 0 {
		return 0
	}
	return float64(p.Files) / p.Duration.Seconds()
}

// Rate estimates remaining time from the last finished build, scaled by how
// fast this session is compiling.
type Rate struct {
	samples []sample
	inst    []float64
	prior   Prior
	eta     time.Duration
	used    float64
	at      time.Time
	ok      bool
}

func NewRate() *Rate { return &Rate{} }

// UsePrior sets the last finished build. Zero means there isn't one yet.
func (r *Rate) UsePrior(p Prior) {
	if p.Files > 0 && p.Duration > 0 {
		r.prior = p
	}
}

// HasPrior reports whether a last run is in use.
func (r *Rate) HasPrior() bool {
	return r.prior.Files > 0 && r.prior.Duration > 0
}

// Note records one observation. elapsed is how long this build has been
// running, not how long the status screen has been open.
// pct is the tracker's current percent. It is only used when no last run exists.
func (r *Rate) Note(now time.Time, compiled, pct int, elapsed time.Duration) (time.Duration, bool) {
	r.noteSpark(now, compiled)
	speed, left, ok := r.remaining(compiled, pct, elapsed)
	if !ok {
		if r.ok && pct < 98 && now.Sub(r.at) < 15*time.Second {
			return r.eta, true
		}
		r.ok = false
		return 0, false
	}
	r.used = speed
	r.eta = time.Duration(float64(left) / speed * float64(time.Second))
	r.at = now
	r.ok = true
	return r.eta, true
}

// remaining is leftover compile lines and the files/sec to use against them.
// With a last run, the leftover is that run's file count minus this one.
// Speed is the last run's average until this session has a real average,
// then this session's average.
func (r *Rate) remaining(compiled, pct int, elapsed time.Duration) (speed float64, left int, ok bool) {
	if pct >= 98 {
		return 0, 0, false
	}
	session := 0.0
	if elapsed >= trustAfter && compiled > 0 {
		session = float64(compiled) / elapsed.Seconds()
	}
	if last := r.prior.speed(); last > 0 {
		speed = last
		if session > 0 {
			speed = session
		}
		left = r.prior.Files - compiled
		if left > 0 && speed > 0 {
			return speed, left, true
		}
	}
	if session <= 0 || pct <= 0 {
		return 0, 0, false
	}
	total := compiled * 100 / pct
	left = total - compiled
	if left <= 0 {
		return 0, 0, false
	}
	return session, left, true
}

func (r *Rate) noteSpark(now time.Time, compiled int) {
	r.samples = append(r.samples, sample{t: now, compiled: compiled})
	cut := now.Add(-rateWindow)
	i := 0
	for i < len(r.samples)-1 && r.samples[i].t.Before(cut) {
		i++
	}
	r.samples = r.samples[i:]
	if len(r.samples) < 2 {
		return
	}
	a := r.samples[0]
	b := r.samples[len(r.samples)-1]
	dt := b.t.Sub(a.t)
	if dt < 2*time.Second {
		return
	}
	dc := b.compiled - a.compiled
	if dc <= 0 {
		return
	}
	per := float64(dc) / dt.Seconds()
	if per <= 0 {
		return
	}
	r.inst = append(r.inst, per)
	if len(r.inst) > 12 {
		r.inst = r.inst[len(r.inst)-12:]
	}
}

// Last is the ETA from the most recent Note that had one.
func (r *Rate) Last() (time.Duration, bool) {
	if !r.ok {
		return 0, false
	}
	return r.eta, true
}

// PerSec is the files/sec used for the ETA. It is 0 until an ETA exists.
func (r *Rate) PerSec() float64 { return r.used }

// Spark is the recent files-per-second shape, oldest to newest.
func (r *Rate) Spark() []float64 {
	out := make([]float64, len(r.inst))
	copy(out, r.inst)
	return out
}
