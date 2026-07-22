package events

import (
	"fmt"
	"testing"
)

func TestRingCapacityAndOrder(t *testing.T) {
	b := New()
	for i := 0; i < Capacity+50; i++ {
		b.Add("u1", "acc", "账号", KindInfo, fmt.Sprintf("e%d", i))
	}
	list := b.List("u1", MaxLimit)
	if len(list) != Capacity {
		t.Fatalf("expected %d events, got %d", Capacity, len(list))
	}
	// 倒序：最新在前；最旧的 50 条已被挤出
	if list[0].Detail != fmt.Sprintf("e%d", Capacity+49) {
		t.Fatalf("newest mismatch: %s", list[0].Detail)
	}
	if list[Capacity-1].Detail != "e50" {
		t.Fatalf("oldest mismatch: %s", list[Capacity-1].Detail)
	}
}

func TestUIDFilterAndLimit(t *testing.T) {
	b := New()
	b.Add("u1", "a", "A", KindInfo, "one")
	b.Add("u2", "a", "A", KindInfo, "two")
	b.Add("u1", "a", "A", KindInfo, "three")

	if got := b.List("u1", 0); len(got) != 2 || got[0].Detail != "three" || got[1].Detail != "one" {
		t.Fatalf("unexpected: %+v", got)
	}
	if got := b.List("u1", 1); len(got) != 1 || got[0].Detail != "three" {
		t.Fatalf("unexpected limit: %+v", got)
	}
	if got := b.List("u2", 0); len(got) != 1 || got[0].Detail != "two" {
		t.Fatalf("unexpected uid filter: %+v", got)
	}
	if got := b.List("u3", 0); len(got) != 0 {
		t.Fatalf("expected empty, got %+v", got)
	}
}

func TestMonotonicIDs(t *testing.T) {
	b := New()
	e1 := b.Add("u1", "a", "A", KindInfo, "x")
	e2 := b.Add("u1", "a", "A", KindInfo, "y")
	if e2.ID <= e1.ID {
		t.Fatal("event IDs must increase")
	}
}
