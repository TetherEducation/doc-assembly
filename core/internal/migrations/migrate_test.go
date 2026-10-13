package migrations

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestLatestIsTheNewestEmbeddedMigration(t *testing.T) {
	latest, err := Latest()
	if err != nil {
		t.Fatal(err)
	}

	entries, err := fs.ReadDir(sqlFiles, "sql")
	if err != nil {
		t.Fatal(err)
	}
	prefix := fmt.Sprintf("%06d_", latest)
	var up, down bool
	for _, entry := range entries {
		name := entry.Name()
		up = up || (strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".up.sql"))
		down = down || (strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".down.sql"))
		if version, err := strconv.ParseUint(strings.SplitN(name, "_", 2)[0], 10, 64); err == nil && uint(version) > latest {
			t.Fatalf("%s is newer than Latest()=%d", name, latest)
		}
	}
	if !up || !down {
		t.Fatalf("Latest()=%d but its up/down files are not both embedded (up=%v down=%v)", latest, up, down)
	}
}

// fakeRows answers ReadState's two queries in order.
type fakeRows struct {
	scans []func(dest ...any) error
	calls int
}

func (f *fakeRows) QueryRow(context.Context, string, ...any) pgx.Row {
	scan := f.scans[f.calls]
	f.calls++
	return rowFunc(scan)
}

type rowFunc func(dest ...any) error

func (r rowFunc) Scan(dest ...any) error { return r(dest...) }

func tableExists(exists bool) func(dest ...any) error {
	return func(dest ...any) error { *dest[0].(*bool) = exists; return nil }
}

func versionRow(version int64, dirty bool) func(dest ...any) error {
	return func(dest ...any) error {
		*dest[0].(*int64), *dest[1].(*bool) = version, dirty
		return nil
	}
}

func TestReadState(t *testing.T) {
	boom := errors.New("connection reset")
	for _, tc := range []struct {
		name    string
		scans   []func(dest ...any) error
		want    State
		wantErr error
	}{
		{"no table", []func(...any) error{tableExists(false)}, State{}, nil},
		{"empty table", []func(...any) error{tableExists(true), func(...any) error { return pgx.ErrNoRows }}, State{}, nil},
		{"first migration never completed", []func(...any) error{tableExists(true), versionRow(-1, true)}, State{Dirty: true}, nil},
		{"clean", []func(...any) error{tableExists(true), versionRow(23, false)}, State{Version: 23, Initialized: true}, nil},
		{"dirty", []func(...any) error{tableExists(true), versionRow(24, true)}, State{Version: 24, Dirty: true, Initialized: true}, nil},
		{"existence query fails", []func(...any) error{func(...any) error { return boom }}, State{}, boom},
		{"version query fails", []func(...any) error{tableExists(true), func(...any) error { return boom }}, State{}, boom},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ReadState(context.Background(), &fakeRows{scans: tc.scans})
			if !errors.Is(err, tc.wantErr) || got != tc.want {
				t.Fatalf("got %+v, %v; want %+v, %v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}
