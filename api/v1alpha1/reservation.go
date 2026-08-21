/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2026 IBM Corp.                                      |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package v1alpha1

import (
	"slices"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This file holds the helpers every component must use to manipulate
// SpyreNodeState.Status.Reservations.
//
// Reservations used to be two unkeyed parallel lists (PodsUnderScheduling and
// DeviceSets) with no link between them, which forced the scheduler, the device
// plugin and the pod watcher to each guess which device set belonged to which
// Pod - usually by matching the number of devices. When several Pods each
// reserve a single device those guesses are indistinguishable from one another,
// so releasing one Pod's reservation could free a different Pod's device and let
// it be handed out twice.
//
// ReservationEntry removes the guessing by binding a Pod (with its UID) to its
// devices. Keeping the manipulation in one place here is what stops the
// cardinality guessing from being reinvented in each repository.

// SameAs reports whether p and q identify the same Pod.
//
// When both sides carry a UID the UIDs must match, so a Pod that reuses the name
// of a predecessor is not mistaken for it. Reservations written by a component
// that predates the UID field have none, in which case name and namespace are
// all there is to compare.
func (p Pod) SameAs(q Pod) bool {
	if p.Name != q.Name || p.Namespace != q.Namespace {
		return false
	}
	if p.UID == "" || q.UID == "" {
		return true
	}
	return p.UID == q.UID
}

// IsDifferentGeneration reports whether p and q share a name but are provably
// distinct Pods, i.e. both carry a UID and the UIDs differ.
func (p Pod) IsDifferentGeneration(q Pod) bool {
	return p.Name == q.Name && p.Namespace == q.Namespace &&
		p.UID != "" && q.UID != "" && p.UID != q.UID
}

// ReservedBefore reports whether the reservation is at least age old, and is how
// callers apply a grace period before releasing a reservation whose Pod has
// vanished from the API. An entry with no ReservedAt (one written by an older
// component, or restored from the deprecated fields) is treated as old enough to
// release immediately - it cannot be newer than the current reconcile.
func (e ReservationEntry) ReservedBefore(now metav1.Time, age time.Duration) bool {
	if e.ReservedAt == nil {
		return true
	}
	return !e.ReservedAt.Add(age).After(now.Time)
}

// deviceSetKey returns an order-insensitive key for a device set, so that two
// reservations of the same devices compare equal regardless of the order the
// writer happened to produce them in.
func deviceSetKey(devices []string) string {
	sorted := slices.Clone(devices)
	slices.Sort(sorted)
	return strings.Join(sorted, "\x00")
}

// Upsert records devices as reserved for pod, replacing any reservation pod
// already held in this resource pool. Reserving no devices removes pod's
// reservation instead, so that an entry never exists without devices.
func (r *Reservation) Upsert(pod Pod, devices []string, reservedAt metav1.Time) {
	r.removeByPod(pod)
	if len(devices) > 0 {
		at := reservedAt
		r.Entries = append(r.Entries, ReservationEntry{
			Pod:        pod,
			DeviceList: slices.Clone(devices),
			ReservedAt: &at,
		})
	}
	r.SyncLegacy()
}

// RemoveByPod drops the reservation held by pod, reporting whether anything was
// removed. Identity follows Pod.SameAs, so a Pod that merely reuses a name does
// not release its predecessor's devices.
func (r *Reservation) RemoveByPod(pod Pod) bool {
	removed := r.removeByPod(pod)
	if removed {
		r.SyncLegacy()
	}
	return removed
}

func (r *Reservation) removeByPod(pod Pod) bool {
	kept := make([]ReservationEntry, 0, len(r.Entries))
	for _, e := range r.Entries {
		if e.Pod.SameAs(pod) {
			continue
		}
		kept = append(kept, e)
	}
	if len(kept) == len(r.Entries) {
		return false
	}
	r.Entries = kept
	return true
}

// RemoveByDevices drops the reservation covering exactly devices, reporting
// whether anything was removed. The device plugin uses this to retire a
// reservation it has just turned into an allocation, in the case where it cannot
// tell which Pod the devices went to.
func (r *Reservation) RemoveByDevices(devices []string) bool {
	want := deviceSetKey(devices)
	for i, e := range r.Entries {
		if deviceSetKey(e.DeviceList) == want {
			r.Entries = slices.Delete(r.Entries, i, i+1)
			r.SyncLegacy()
			return true
		}
	}
	return false
}

// EntryForPod returns the reservation held by pod.
func (r Reservation) EntryForPod(pod Pod) (ReservationEntry, bool) {
	for _, e := range r.Entries {
		if e.Pod.SameAs(pod) {
			return e, true
		}
	}
	return ReservationEntry{}, false
}

// EntryForSize returns a reservation of exactly n devices, all of which appear
// in available.
//
// This is the one place where a reservation is still matched without knowing the
// Pod, because the kubelet device plugin API does not tell GetPreferredAllocation
// which Pod it is allocating for. It is safe: any two reservations of the same
// size are interchangeable at that point - each waiting Pod asked for that many
// devices and each candidate set is equally valid for it, so handing Pod A the
// set recorded for Pod B still gives both Pods the right number of distinct
// devices. What was not safe was doing the same guess where the Pod *is* known,
// which is why the scheduler no longer does it.
func (r Reservation) EntryForSize(n int, available []string) (ReservationEntry, bool) {
	for _, e := range r.Entries {
		if len(e.DeviceList) != n {
			continue
		}
		if !allIn(e.DeviceList, available) {
			continue
		}
		return e, true
	}
	return ReservationEntry{}, false
}

func allIn(devices, available []string) bool {
	for _, d := range devices {
		if !slices.Contains(available, d) {
			return false
		}
	}
	return true
}

// ReservedDevices returns every device reserved in this resource pool.
func (r Reservation) ReservedDevices() []string {
	devices := make([]string, 0, len(r.Entries))
	for _, e := range r.Entries {
		devices = append(devices, e.DeviceList...)
	}
	return devices
}

// IsEmpty reports whether the reservation holds nothing.
func (r Reservation) IsEmpty() bool {
	return len(r.Entries) == 0
}

// SyncLegacy rebuilds the deprecated PodsUnderScheduling and DeviceSets fields
// from Entries. Every write path must leave them consistent so that a component
// which has not yet been updated still sees a complete picture during a rolling
// upgrade. All the mutators here call it for you.
func (r *Reservation) SyncLegacy() {
	if len(r.Entries) == 0 {
		r.PodsUnderScheduling = nil
		r.DeviceSets = nil
		return
	}
	pods := make([]Pod, 0, len(r.Entries))
	sets := make([][]string, 0, len(r.Entries))
	for _, e := range r.Entries {
		pods = append(pods, e.Pod)
		sets = append(sets, slices.Clone(e.DeviceList))
	}
	r.PodsUnderScheduling = pods
	r.DeviceSets = sets
}

// NormalizeFromLegacy reconciles Entries with the deprecated fields, and must be
// called on every Reservation read from the API before it is inspected or
// modified. It is what lets an updated component share a SpyreNodeState with one
// that still writes only PodsUnderScheduling and DeviceSets.
//
// DeviceSets is taken as the authority on which devices are reserved, since that
// is the field an older writer edits and getting it wrong risks handing out a
// device twice. Entries whose devices are still listed there survive with their
// Pod identity intact; device sets with no matching entry are adopted as new
// entries, taking an owner from PodsUnderScheduling when one is left over and
// otherwise recording no owner at all. An entry whose devices have disappeared
// from DeviceSets was retired by the older writer and is dropped.
//
// An entry adopted this way has no ReservedAt and, if no owner was left to pair
// it with, no Pod either. Callers must therefore keep handling entries with an
// empty Pod - they still reserve devices, they just cannot be matched to a Pod
// by identity.
func (r *Reservation) NormalizeFromLegacy() {
	// Fast path: the last writer was up to date, so the deprecated fields are
	// exactly what SyncLegacy would have produced.
	if r.legacyIsInSync() {
		return
	}

	remaining := make([]int, 0, len(r.DeviceSets))
	for i := range r.DeviceSets {
		remaining = append(remaining, i)
	}

	kept := make([]ReservationEntry, 0, len(r.Entries))
	for _, e := range r.Entries {
		want := deviceSetKey(e.DeviceList)
		matched := -1
		for j, idx := range remaining {
			if deviceSetKey(r.DeviceSets[idx]) == want {
				matched = j
				break
			}
		}
		if matched < 0 {
			// The older writer retired this reservation.
			continue
		}
		remaining = slices.Delete(remaining, matched, matched+1)
		kept = append(kept, e)
	}

	// Whatever is left in DeviceSets was added by the older writer. Pair it with
	// any Pod that no surviving entry claims.
	unclaimed := make([]Pod, 0, len(r.PodsUnderScheduling))
	for _, p := range r.PodsUnderScheduling {
		owned := false
		for _, e := range kept {
			if e.Pod.SameAs(p) {
				owned = true
				break
			}
		}
		if !owned {
			unclaimed = append(unclaimed, p)
		}
	}
	for _, idx := range remaining {
		e := ReservationEntry{DeviceList: slices.Clone(r.DeviceSets[idx])}
		if len(unclaimed) > 0 {
			e.Pod = unclaimed[0]
			unclaimed = unclaimed[1:]
		}
		kept = append(kept, e)
	}

	r.Entries = kept
	r.SyncLegacy()
}

func (r Reservation) legacyIsInSync() bool {
	if len(r.Entries) != len(r.DeviceSets) || len(r.Entries) != len(r.PodsUnderScheduling) {
		return false
	}
	for i, e := range r.Entries {
		if e.Pod != r.PodsUnderScheduling[i] {
			return false
		}
		if !slices.Equal(e.DeviceList, r.DeviceSets[i]) {
			return false
		}
	}
	return true
}

// --- SpyreNodeStateStatus level helpers -------------------------------------
//
// Reservations is a map of struct values, so a caller that mutates a Reservation
// in place has to remember to store it back. These wrappers do that once, here,
// instead of at every call site.

// NormalizeReservations prepares every reservation for inspection. Call it right
// after reading a SpyreNodeState. Reservations left empty - including the empty
// placeholders older schedulers wrote - are dropped.
func (s *SpyreNodeStateStatus) NormalizeReservations() {
	for name, r := range s.Reservations {
		r.NormalizeFromLegacy()
		if r.IsEmpty() {
			delete(s.Reservations, name)
			continue
		}
		s.Reservations[name] = r
	}
}

// ReserveDevices records devices as reserved for pod in the resourceName pool,
// replacing any reservation pod already held there.
func (s *SpyreNodeStateStatus) ReserveDevices(
	resourceName string, pod Pod, devices []string, reservedAt metav1.Time) {
	if s.Reservations == nil {
		s.Reservations = make(map[string]Reservation)
	}
	r := s.Reservations[resourceName]
	r.Upsert(pod, devices, reservedAt)
	if r.IsEmpty() {
		delete(s.Reservations, resourceName)
		return
	}
	s.Reservations[resourceName] = r
}

// ReleaseReservation drops every reservation held by pod, in any resource pool,
// reporting whether anything was removed.
func (s *SpyreNodeStateStatus) ReleaseReservation(pod Pod) bool {
	removed := false
	for name, r := range s.Reservations {
		if !r.RemoveByPod(pod) {
			continue
		}
		removed = true
		if r.IsEmpty() {
			delete(s.Reservations, name)
			continue
		}
		s.Reservations[name] = r
	}
	return removed
}

// ReservedDevices returns every device reserved on this node, across all
// resource pools. A device is unavailable if it appears here or in
// AllocationList.
func (s SpyreNodeStateStatus) ReservedDevices() []string {
	devices := []string{}
	for _, r := range s.Reservations {
		devices = append(devices, r.ReservedDevices()...)
	}
	return devices
}

// AllocatedDevices returns every device already allocated on this node.
func (s SpyreNodeStateStatus) AllocatedDevices() []string {
	devices := []string{}
	for _, a := range s.AllocationList {
		devices = append(devices, a.DeviceList...)
	}
	return devices
}

// ReservationEntries returns every reservation entry on this node paired with
// the resource pool holding it, which is what a caller auditing reservations
// across pools needs.
func (s SpyreNodeStateStatus) ReservationEntries() map[string][]ReservationEntry {
	entries := make(map[string][]ReservationEntry, len(s.Reservations))
	for name, r := range s.Reservations {
		entries[name] = r.Entries
	}
	return entries
}
