package store

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func Test_store_numbered_names_sort_by_number(t *testing.T) {
	var names []string
	for _, n := range []int{10, 9, 1, 100, 2, 99999} {
		names = append(names, Step("ctl", "c1/i", n), Emit("ctl", "c1/i", 3, n))
	}
	slices.Sort(names)
	last := map[bool]int{}
	for _, name := range names {
		emit := strings.Contains(name, "/emits/")
		if n := Number(name); n <= last[emit] {
			t.Fatalf("%s sorts after number %d", name, last[emit])
		} else {
			last[emit] = n
		}
	}
}

func Test_store_wake_names_sort_by_time(t *testing.T) {
	base := time.Date(2026, 10, 10, 9, 59, 59, 5e6, time.FixedZone("far", 11*3600))
	times := []time.Time{base, base.Add(time.Millisecond), base.Add(995 * time.Millisecond).In(time.UTC),
		base.Add(time.Hour).In(time.FixedZone("west", -8*3600)), base.Add(300 * 24 * time.Hour)}
	var names []string
	for i := len(times) - 1; i >= 0; i-- {
		names = append(names, Wake("ctl", times[i], "c1/i", "timer.x"))
	}
	slices.Sort(names)
	for i, name := range names {
		at, instance, what, ok := ParseWake("ctl", name)
		if !ok || !at.Equal(times[i].Truncate(time.Millisecond)) || instance != "c1/i" || what != "timer.x" {
			t.Fatalf("wake %d is %s: %v %s %s", i, name, at, instance, what)
		}
	}
}

func Test_store_message_names_sort_in_order_sent(t *testing.T) {
	var s Sender
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	var names []string
	for i := range 1000 {
		at := now
		if i%3 == 0 {
			at = now.Add(-time.Duration(i) * time.Second) // the clock steps back
		}
		names = append(names, s.Next(at))
	}
	if !slices.IsSorted(names) || len(slices.Compact(slices.Clone(names))) != len(names) {
		t.Fatal("names do not sort in the order sent")
	}
}

func Test_store_no_name_is_a_path_above_another(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	i := NewInstance("c1", now)
	child := ChildInstance(i, now, "fetch.0")
	names := []string{
		VersionName, Message("ctl", "c1", "m1"), Receipt("ctl", "c1", "m1"),
		RunningHead("ctl", i), FinishedHead("ctl", i), RunningHead("ctl", child),
		Step("ctl", i, 1), Emit("ctl", i, 1, 1), Blob("ctl", i, "b1"), Step("ctl", child, 1),
		Wake("ctl", now, i, "timer.x"), Trust("ctl", "r1"),
		Message("r1", "ctl", JobMessage(i, "fetch")), Taken("r1", now, JobMessage(i, "fetch")),
		Message("ctl", "ctl", LinkMessage(i, 3, 0, "start")),
		ReceiptBlob("ctl", "c1", "m1", "b1"), Result("ctl", "c1", "m1"), Upload("ctl", "c1", "m2", "f1"),
		Tree("ctl", "/home"), LocationFile("ctl", "/home/boiler"), LocationBlob("ctl", "/home/boiler", "b1"),
		Key("ctl", "/home/boiler", ""), Key("ctl", "/home/rooms", "k"),
		LocationFile("ctl", "/project/.rook/review"), LocationFile("ctl", "/project/.rook"),
		Secret("ctl", "/", "KEY"), Secret("ctl", "/home", "KEY"), Secret("ctl", "/home", "+"),
		Value("ctl", "/", "HUB"), Value("ctl", "/home/boiler", "HUB"),
		// A location may be named as an entry's kind is, under a prefix
		// that holds entries.
		LocationFile("ctl", "/home/secrets"), LocationFile("ctl", "/home/tree"), LocationFile("ctl", "/file"),
	}
	for _, a := range names {
		if err := checkName(a); err != nil {
			t.Error(err)
		}
		for _, b := range names {
			if strings.HasPrefix(b, a+"/") {
				t.Errorf("%s is a path above %s", a, b)
			}
		}
	}
	if CheckInstance(i) != nil || CheckInstance(child) != nil || CheckInstance("c1/../x") == nil || CheckInstance("x") == nil {
		t.Fatal("instance names are not checked")
	}
	if InstanceClient(child) != "c1" || InstanceOf("ctl", Step("ctl", child, 4)) != child || InstanceOf("ctl", RunningHead("ctl", i)) != i {
		t.Fatal("an instance's name does not say where it is")
	}
	if at, err := InstanceTime(child); err != nil || !at.Equal(now) {
		t.Fatalf("an instance's name does not carry its time: %v %v", at, err)
	}
	if ChildInstance(i, now, "fetch.0") != child || ChildInstance(i, now, "fetch.1") == child {
		t.Fatal("a child's name does not follow from what asked for it")
	}
}

// A location's entries are under "+", and no part of a location is "+": so
// no location's entries are a path above another's. A part that starts
// with a dot is kept after a "#", which no part of a location holds.
func Test_store_location_part_cannot_be_plus(t *testing.T) {
	for _, location := range []string{"/", "/home", "/home/devices/boiler", "/a-b_c/d:e", "/project/.rook/review", "/.x", "/...", "/a b/%20"} {
		if err := CheckLocation(location); err != nil {
			t.Errorf("%s: %v", location, err)
		}
	}
	for _, location := range []string{"", "home", "home/x", "/home/", "//home", "/home//x", "/+", "/home/+",
		"/home/+/secrets", "/home/.", "/home/..", "/home/../x", "/.", "/a#b", "/#.rook", "/home/a\\b", "/a\nb", "/.a\nb", "/sta*", "/home/*", "/home/**", "/a*b/c"} {
		if err := CheckLocation(location); err == nil {
			t.Errorf("%q was taken as a location", location)
		}
	}
	for _, location := range []string{"/", "/home", "/home/devices/boiler", "/project/.rook/review"} {
		for _, name := range []string{LocationFile("ctl", location), Secret("ctl", location, "K"), Key("ctl", location, "k")} {
			if at, _, ok := ParseLocation("ctl", name); !ok || at != location {
				t.Errorf("%s is read as at %q", name, at)
			}
		}
	}
	if at, entry, ok := ParseLocation("ctl", Secret("ctl", "/home", "+")); !ok || at != "/home" || entry != "secrets/+" {
		t.Errorf("a secret named + is read as %q %q", at, entry)
	}
	if got := Above("/home/devices/boiler"); !slices.Equal(got, []string{"/home/devices/boiler", "/home/devices", "/home", "/"}) {
		t.Errorf("above /home/devices/boiler is %v", got)
	}
	if got := Above("/"); !slices.Equal(got, []string{"/"}) {
		t.Errorf("above / is %v", got)
	}
	if Secret("ctl", "/", "K") != "ctl/locations/+/secrets/K" || Secret("ctl", "/home", "K") != "ctl/locations/home/+/secrets/K" ||
		LocationFile("ctl", "/project/.rook/review") != "ctl/locations/project/#.rook/review/+/file" {
		t.Error("secrets are not where the layout draws them")
	}
	if message, ok := ReceiptMessage("ctl", ReceiptBlob("ctl", "c1", "m1", "b")); !ok || message != "m1" {
		t.Errorf("a receipt's blob is read as of %q", message)
	}
	if message, ok := ReceiptMessage("ctl", Result("ctl", "c1", "m1")); !ok || message != "m1" {
		t.Errorf("a call's result is read as of %q", message)
	}
}
