// Package backup answers the product's backupNow: a durable copy of the
// session profile.
//
// IT REFUSES TO COPY A PROFILE IN USE, and that refusal is the capability
// rather than a limitation of it. Measured on 2026-08-19 against the lab
// profile: in TEN SECONDS of an IDLE live session, 11 of 1124 files changed or
// appeared. Chromium keeps LevelDB and IndexedDB writing underneath, so a copy
// taken while the browser runs catches files mid-write — and the result is not
// a slightly-stale backup, it is a corrupt one that looks fine until the day it
// is restored.
//
// A BACKUP IS NOT A BACKUP UNTIL IT HAS BEEN RESTORED. This package copies and
// counts; it does not claim the copy is usable, because the only proof of that
// is booting from it. Verify exists for exactly that, and carries a hazard the
// caller must decide about — see its doc.
package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"wa-api/internal/headless/engine"
)

// ErrProfileInUse is a backup refused because a browser holds the profile.
var ErrProfileInUse = errors.New("backup: the profile is in use; a copy taken now would " +
	"catch files mid-write")

// ErrDestinationNotEmpty is a backup refused because the destination already
// has something in it.
//
// Refusing rather than merging: a merge of two profiles is neither profile, and
// the failure would appear as a session that boots and then behaves strangely.
var ErrDestinationNotEmpty = errors.New("backup: the destination directory is not empty")

// Result is what a backup copied.
type Result struct {
	// Files and Bytes are what landed in the destination.
	Files int
	Bytes int64
	// Skipped counts entries that were NOT copied because they are not regular
	// files — sockets and the Singleton symlinks, which name a process and are
	// meaningless in a copy.
	Skipped int
	// Took is the wall time of the copy.
	Took time.Duration
	// Source and Destination are absolute paths.
	Source, Destination string
}

// destOpener creates the destination file for one copied entry.
//
// It exists as a parameter because the guard that matters most here — the error
// from Close — cannot be reached otherwise. Close is where a buffered write
// finally fails, and making it fail on a real filesystem needs a full disk or a
// hostile mount; neither belongs in a test. H27 recorded that guard as PRESENT
// AND UNTESTED for exactly that reason, and this is the seam that closes it.
//
// A parameter rather than a package-level variable: a swappable global would
// race between parallel tests and would be reachable from production, which is
// a larger door than the one being opened.
type destOpener func(path string) (io.WriteCloser, error)

// osDestOpener is the production opener.
func osDestOpener(path string) (io.WriteCloser, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
}

// Backup copies src to dst.
//
// The order is load-bearing: the in-use check happens BEFORE anything is
// written, so a refused backup leaves no half-written directory for someone to
// find later and mistake for a real one.
func Backup(src, dst string) (Result, error) {
	return backupWith(src, dst, osDestOpener)
}

func backupWith(src, dst string, open destOpener) (Result, error) {
	var res Result

	absSrc, err := filepath.Abs(src)
	if err != nil {
		return res, fmt.Errorf("backup: resolving source: %w", err)
	}
	absDst, err := filepath.Abs(dst)
	if err != nil {
		return res, fmt.Errorf("backup: resolving destination: %w", err)
	}
	if absSrc == absDst {
		return res, errors.New("backup: source and destination are the same directory")
	}
	res.Source, res.Destination = absSrc, absDst

	use, err := engine.ProfileInUse(absSrc)
	if err != nil {
		return res, fmt.Errorf("backup: checking whether the profile is in use: %w", err)
	}
	if use.InUse() {
		return res, fmt.Errorf("%w (lock holder %q, pid %d, live %v)",
			ErrProfileInUse, use.Holder, use.PID, use.Live)
	}

	if err := requireEmptyOrAbsent(absDst); err != nil {
		return res, err
	}
	if err := os.MkdirAll(absDst, 0o700); err != nil {
		return res, fmt.Errorf("backup: creating destination: %w", err)
	}

	start := time.Now()
	err = filepath.WalkDir(absSrc, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(absSrc, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(absDst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		// Only regular files. A Singleton lock is a SYMLINK naming a pid, and
		// copying it would carry a claim about a process that does not exist in
		// the restored profile — the exact stale-lock condition ReclaimProfile
		// exists to clean up.
		if !d.Type().IsRegular() {
			res.Skipped++
			return nil
		}
		n, err := copyFile(path, target, open)
		if err != nil {
			return err
		}
		res.Files++
		res.Bytes += n
		return nil
	})
	res.Took = time.Since(start)
	if err != nil {
		return res, fmt.Errorf("backup: copying: %w", err)
	}
	if res.Files == 0 {
		// An empty copy is not a backup, and returning success here would let a
		// wrong source path look like a working one.
		return res, fmt.Errorf("backup: copied no files from %s; the source is empty "+
			"or is not a profile directory", absSrc)
	}
	return res, nil
}

func requireEmptyOrAbsent(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("backup: reading destination: %w", err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("%w: %s has %d entr(ies)", ErrDestinationNotEmpty, dir, len(entries))
	}
	return nil
}

func copyFile(src, dst string, open destOpener) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		// A file that vanished between the walk and the open is a profile that
		// is being written to — which the in-use check was supposed to have
		// ruled out. Reporting it as a copy error rather than skipping it keeps
		// the guarantee honest.
		return 0, fmt.Errorf("opening %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()

	out, err := open(dst)
	if err != nil {
		return 0, fmt.Errorf("creating %s: %w", dst, err)
	}

	// CLOSE IS ALWAYS CALLED, including when the copy failed. Returning early on
	// copyErr would leave the descriptor open and — worse for a backup — would
	// skip the flush whose failure is the thing being guarded against. A copy
	// error and a close error are then reported in that order, because the
	// first one explains the second.
	n, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return n, fmt.Errorf("copying %s: %w", src, copyErr)
	}
	if closeErr != nil {
		// Close is where a buffered write finally fails. Ignoring it is how a
		// truncated file ends up in a backup that reports success.
		return n, fmt.Errorf("closing %s: %w", dst, closeErr)
	}
	return n, nil
}

// Verify proves a backup is restorable by BOOTING it, which is the only proof
// there is: a copy that cannot be booted is not a backup, and file counts say
// nothing about that.
//
// THE HAZARD THE CALLER MUST DECIDE ABOUT. A restored profile carries the SAME
// WhatsApp credentials as the original. Booting it while the original session is
// also live puts two browsers on one account, which is the situation invariant 1
// exists to prevent — at the profile level it is a different directory, so
// nothing in this stack will stop it, and WhatsApp may respond by invalidating
// one of them.
//
// So Verify takes the boot as a function the caller supplies rather than
// booting anything itself: whoever calls it must have decided the original is
// stopped, and making them pass the boot is how that decision becomes explicit
// instead of implied.
func Verify(ctx context.Context, dst string, boot func(context.Context, string) error) error {
	if boot == nil {
		return errors.New("backup: Verify needs a boot function; a backup nobody tried to " +
			"restore is unverified by definition")
	}
	if err := boot(ctx, dst); err != nil {
		return fmt.Errorf("backup: the copy at %s did not boot: %w", dst, err)
	}
	return nil
}
