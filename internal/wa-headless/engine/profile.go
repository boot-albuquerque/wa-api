package engine

// Reclaiming the Singleton files of a persistent profile.
//
// Chromium refuses to start on a profile that still carries SingletonLock,
// SingletonCookie and SingletonSocket from a previous run. After a crash they
// are stale and the next boot needs them gone — the study measured this on
// every recovery trial, and it is invariant 15 of the initiative.
//
// The study removed them unconditionally, and it was right to be suspicious of
// that: ReclaimSingletons was the independent variable of phase 4C section 21,
// because one live hypothesis for losing a paired session after ~9-10 boots was
// that the reclaim itself corrupted the profile. The ablation refuted it — the
// cause was the shutdown form, not the reclaim — so the operation is safe.
//
// What the study also wrote down, and did not enforce, is section 18: the
// reclaim is legitimate ONLY under exclusive ownership. There it was trivially
// true, since one process owned the directory. In production a profile can sit
// on a shared volume, and deleting a lock another process is holding lets two
// browsers into one profile, which is how a session gets destroyed for real.
//
// So this version looks before it deletes.
//
// Study origin: scripts/chromium-study/p3_launcher.go, reclaimProfile.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The three files Chromium leaves behind. Named, never spelled at a call site.
const (
	singletonLock   = "SingletonLock"
	singletonCookie = "SingletonCookie"
	singletonSocket = "SingletonSocket"
)

var singletonFiles = []string{singletonLock, singletonCookie, singletonSocket}

// ErrProfileHeldByLiveBrowser means the lock is NOT stale: a process on this
// host still holds the profile. Reclaiming would put two browsers on one
// session directory.
var ErrProfileHeldByLiveBrowser = errors.New("profile is held by a live browser")

// ErrProfileHeldByAnotherHost means the lock names a host that is not this one,
// so its liveness cannot be judged from here.
//
// Refusing is the default because the failure it prevents is silent and
// permanent — two browsers in one profile — while the failure it causes is
// loud: a boot that does not happen. A caller that holds a lease proving single
// ownership can set ReclaimOptions.AllowForeignHost to override.
var ErrProfileHeldByAnotherHost = errors.New("profile is held by another host")

// ReclaimOptions tunes what the reclaim is willing to assume.
type ReclaimOptions struct {
	// AllowForeignHost permits reclaiming a lock stamped with a different
	// hostname. Only set this where single ownership is proven by something
	// else — a KV lease with a fencing token, not a guess.
	AllowForeignHost bool
	// Hostname overrides the detected hostname. For tests.
	Hostname string
	// ProcessAlive reports whether a pid is running on this host.
	//
	// REQUIRED. There is no default, and that is deliberate: the reclaim must
	// never guess that a holder is dead. A nil probe cannot prove death, so it
	// refuses instead — failing closed, because the failure it prevents (two
	// browsers in one profile) is silent and permanent while the failure it
	// causes is a boot that loudly does not happen.
	//
	// The launcher supplies the real probe; it is the layer that knows about
	// processes.
	ProcessAlive func(pid int) bool
}

// ReclaimResult reports what happened, because "the boot worked" and "the boot
// worked because we deleted somebody else's lock" must not look the same in a
// log.
type ReclaimResult struct {
	// Removed lists the files actually deleted.
	Removed []string
	// LockHolder is what the lock claimed, when it could be read.
	LockHolder string
	// Reason states why removal was allowed.
	Reason string
}

// ReclaimProfile clears the Singleton files of dir when it can prove they are
// stale.
//
// A missing directory is not an error: a profile that has never booted has
// nothing to reclaim.
func ReclaimProfile(dir string, opts ReclaimOptions) (ReclaimResult, error) {
	var res ReclaimResult

	holder, err := readLockHolder(dir)
	if err != nil {
		return res, err
	}
	res.LockHolder = holder.raw

	reason, err := reclaimVerdict(holder, opts)
	if err != nil {
		return res, err
	}
	res.Reason = reason

	for _, name := range singletonFiles {
		p := filepath.Join(dir, name)
		if _, err := os.Lstat(p); err != nil {
			continue
		}
		if err := os.Remove(p); err != nil {
			return res, fmt.Errorf("reclaim %s: %w", name, err)
		}
		res.Removed = append(res.Removed, name)
	}
	return res, nil
}

// ProfileUse is what a profile's Singleton lock says about who is using it.
type ProfileUse struct {
	// LockPresent is whether SingletonLock exists at all.
	LockPresent bool
	// Holder is the raw lock target ("<host>-<pid>"), for diagnosis.
	Holder string
	// PID is the process the lock names, when it could be parsed.
	PID int
	// Live is whether that process is still running ON THIS HOST.
	//
	// A lock naming another host's pid cannot be checked from here, and Live is
	// false in that case — which is NOT the same as "the profile is free". The
	// caller must treat an unparsed or foreign lock as unknown rather than
	// absent; LockPresent is what says something is there.
	Live bool
}

// InUse reports whether the profile should be treated as busy: a lock exists
// and either names a live local process or could not be attributed at all.
//
// Erring toward "busy" is deliberate. The cost of being wrong is asymmetric:
// treating a free profile as busy delays a copy, while treating a busy one as
// free produces a snapshot of files mid-write and calls it a backup.
func (u ProfileUse) InUse() bool {
	if !u.LockPresent {
		return false
	}
	return u.Live || u.PID == 0
}

// ProfileInUse reads the Singleton lock of dir and reports who holds it.
//
// It exists because capabilities/backup needs the question answered and must
// not learn the Singleton file layout to ask it — that layout is this package's
// knowledge (ADR-0006 D1's boundary applied to profile internals, not only to
// the driver).
func ProfileInUse(dir string) (ProfileUse, error) {
	h, err := readLockHolder(dir)
	if err != nil {
		return ProfileUse{}, err
	}
	use := ProfileUse{LockPresent: h.present, Holder: h.raw}
	if h.parsed {
		use.PID = h.pid
		use.Live = ProcessAlive(h.pid)
	}
	return use, nil
}

// lockHolder is what SingletonLock claims about its owner.
type lockHolder struct {
	present bool
	parsed  bool
	raw     string
	host    string
	pid     int
}

// readLockHolder reads SingletonLock, which Chromium writes as a SYMLINK whose
// target is "<hostname>-<pid>" (chrome/browser/process_singleton_posix.cc).
//
// It is read with Readlink, never with Stat-then-Open: the target names a
// process, not a path, so the symlink is dangling by design and following it
// would report "no such file" for a lock that exists.
func readLockHolder(dir string) (lockHolder, error) {
	var h lockHolder

	p := filepath.Join(dir, singletonLock)
	if _, err := os.Lstat(p); err != nil {
		if os.IsNotExist(err) {
			return h, nil
		}
		return h, fmt.Errorf("stat %s: %w", singletonLock, err)
	}
	h.present = true

	target, err := os.Readlink(p)
	if err != nil {
		return h, nil // present but not a symlink: unreadable, not fatal
	}
	h.raw = target

	// The pid is after the LAST hyphen: a hostname may contain hyphens.
	i := strings.LastIndex(target, "-")
	if i <= 0 {
		return h, nil
	}
	pid, err := strconv.Atoi(target[i+1:])
	if err != nil || pid <= 0 {
		return h, nil
	}
	h.parsed = true
	h.host = target[:i]
	h.pid = pid
	return h, nil
}

func hostnameOf(opts ReclaimOptions) string {
	if opts.Hostname != "" {
		return opts.Hostname
	}
	name, err := os.Hostname()
	if err != nil {
		// An unknown hostname can never equal the lock's, so every lock reads
		// as foreign and the reclaim refuses. Failing closed is right here:
		// the alternative is deleting a lock we cannot attribute.
		return ""
	}
	return name
}

// reclaimVerdict decides whether the Singleton files may go, and why.
//
// Separated from the deletion so that the DECISION can be read on its own: it
// is the part that must never be permissive, and the part where a wrong answer
// puts two browsers on one profile.
func reclaimVerdict(holder lockHolder, opts ReclaimOptions) (string, error) {
	switch {
	case !holder.present:
		return "no lock present", nil

	case !holder.parsed:
		// An unreadable lock cannot prove a live holder, and Chromium will
		// refuse to boot while it exists. Removing it is the only way forward,
		// and saying so is better than a silent delete.
		return "lock is unreadable; no live holder could be proven", nil

	case holder.host != hostnameOf(opts):
		if !opts.AllowForeignHost {
			return "", fmt.Errorf("%w: lock names host %q, this is %q; reclaiming would risk "+
				"two browsers on one profile. Prove single ownership and set AllowForeignHost",
				ErrProfileHeldByAnotherHost, holder.host, hostnameOf(opts))
		}
		return "foreign host, reclaim explicitly authorised by the caller", nil

	case opts.ProcessAlive == nil:
		return "", fmt.Errorf("%w: pid %d on this host, and no liveness probe was supplied "+
			"to prove otherwise", ErrProfileHeldByLiveBrowser, holder.pid)

	case opts.ProcessAlive(holder.pid):
		return "", fmt.Errorf("%w: pid %d on %q is still running", ErrProfileHeldByLiveBrowser,
			holder.pid, holder.host)

	default:
		return fmt.Sprintf("holder pid %d on this host is gone", holder.pid), nil
	}
}
