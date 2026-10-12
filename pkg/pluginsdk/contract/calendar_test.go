package contract_test

import (
	calendar "github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginproto/bloem/calendar/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"testing"
)

func TestCalendarPreservesAllDayAndTimedEvents(t *testing.T) {
	input := &calendar.CalendarPage{SnapshotId: "snapshot", VisibilityDigest: "scope", Complete: true, Events: []*calendar.CalendarEvent{
		{EventId: "film:release", ContentId: "film", Date: "2026-03-29", Favourite: true},
		{EventId: "episode:airing", ContentId: "episode", Date: "2026-03-29", AirAt: timestamppb.New(timestamppb.Now().AsTime()), AirTimezone: "Europe/Amsterdam", Followed: true},
	}}
	raw, err := proto.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	out := new(calendar.CalendarPage)
	if err = proto.Unmarshal(raw, out); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(input, out) {
		t.Fatal("snapshot did not round-trip")
	}
	if out.Events[0].AirAt != nil {
		t.Fatal("all-day event gained an invented airing time")
	}
	if out.Events[1].AirAt == nil || out.Events[1].AirTimezone != "Europe/Amsterdam" {
		t.Fatal("timed event lost its source timezone")
	}
	service := calendar.File_bloem_calendar_v1_calendar_proto.Services().ByName("CalendarHost")
	if service == nil || service.FullName() != "bloem.calendar.v1.CalendarHost" || service.Methods().ByName("Read") == nil || service.Methods().ByName("CheckAuthority") == nil {
		t.Fatal("optional installation-bound calendar service missing")
	}
}
