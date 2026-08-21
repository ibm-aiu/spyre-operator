/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2026 IBM Corp.                                      |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package v1alpha1_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
)

var now = metav1.NewTime(time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC))

func pod(name, uid string) spyrev1alpha1.Pod {
	return spyrev1alpha1.Pod{Name: name, Namespace: "myapp", UID: k8stypes.UID(uid)}
}

var _ = Describe("Pod identity", func() {
	It("matches on UID when both sides have one", func() {
		Expect(pod("app1", "uid-a").SameAs(pod("app1", "uid-a"))).To(BeTrue())
		Expect(pod("app1", "uid-a").SameAs(pod("app1", "uid-b"))).To(BeFalse())
	})

	It("falls back to name and namespace when a UID is missing", func() {
		// A reservation written before the UID field existed must still be
		// matchable, otherwise it could never be released.
		Expect(pod("app1", "").SameAs(pod("app1", "uid-a"))).To(BeTrue())
		Expect(pod("app1", "uid-a").SameAs(pod("app1", ""))).To(BeTrue())
		Expect(pod("app1", "").SameAs(pod("app2", ""))).To(BeFalse())
	})

	It("recognises a different generation only when both UIDs are known", func() {
		Expect(pod("app1", "uid-a").IsDifferentGeneration(pod("app1", "uid-b"))).To(BeTrue())
		Expect(pod("app1", "uid-a").IsDifferentGeneration(pod("app1", "uid-a"))).To(BeFalse())
		Expect(pod("app1", "").IsDifferentGeneration(pod("app1", "uid-b"))).To(BeFalse())
		Expect(pod("app1", "uid-a").IsDifferentGeneration(pod("app2", "uid-b"))).To(BeFalse())
	})
})

var _ = Describe("ReservationEntry grace period", func() {
	It("treats an entry with no timestamp as releasable", func() {
		e := spyrev1alpha1.ReservationEntry{}
		Expect(e.ReservedBefore(now, 30*time.Second)).To(BeTrue())
	})

	It("keeps a fresh entry until the grace period passes", func() {
		reserved := metav1.NewTime(now.Add(-10 * time.Second))
		e := spyrev1alpha1.ReservationEntry{ReservedAt: &reserved}
		Expect(e.ReservedBefore(now, 30*time.Second)).To(BeFalse())
		Expect(e.ReservedBefore(now, 10*time.Second)).To(BeTrue())
		Expect(e.ReservedBefore(now, 5*time.Second)).To(BeTrue())
	})
})

var _ = Describe("Reservation mutation", func() {
	It("binds each Pod to its own devices and keeps the deprecated fields in step", func() {
		r := spyrev1alpha1.Reservation{}
		r.Upsert(pod("app1", "uid-a"), []string{"000a"}, now)
		r.Upsert(pod("app2", "uid-b"), []string{"000b"}, now)

		Expect(r.Entries).To(HaveLen(2))
		Expect(r.DeviceSets).To(Equal([][]string{{"000a"}, {"000b"}}))
		Expect(r.PodsUnderScheduling).To(Equal([]spyrev1alpha1.Pod{pod("app1", "uid-a"), pod("app2", "uid-b")}))
	})

	It("replaces a Pod's own reservation rather than adding a second one", func() {
		r := spyrev1alpha1.Reservation{}
		r.Upsert(pod("app1", "uid-a"), []string{"000a"}, now)
		r.Upsert(pod("app1", "uid-a"), []string{"000c"}, now)

		Expect(r.Entries).To(HaveLen(1))
		Expect(r.Entries[0].DeviceList).To(Equal([]string{"000c"}))
	})

	It("keeps a same-named Pod of a different generation separate", func() {
		r := spyrev1alpha1.Reservation{}
		r.Upsert(pod("app1", "uid-a"), []string{"000a"}, now)
		r.Upsert(pod("app1", "uid-b"), []string{"000b"}, now)

		Expect(r.Entries).To(HaveLen(2))
	})

	It("reserving no devices removes the entry", func() {
		r := spyrev1alpha1.Reservation{}
		r.Upsert(pod("app1", "uid-a"), []string{"000a"}, now)
		r.Upsert(pod("app1", "uid-a"), nil, now)

		Expect(r.Entries).To(BeEmpty())
		Expect(r.DeviceSets).To(BeNil())
		Expect(r.PodsUnderScheduling).To(BeNil())
	})

	// This is the behaviour the old size-matching code got wrong: with several
	// single-device reservations it could release the wrong Pod's device.
	It("releases only the named Pod's devices when every reservation is the same size", func() {
		r := spyrev1alpha1.Reservation{}
		r.Upsert(pod("app1", "uid-a"), []string{"000a"}, now)
		r.Upsert(pod("app2", "uid-b"), []string{"000b"}, now)
		r.Upsert(pod("app3", "uid-c"), []string{"000c"}, now)

		Expect(r.RemoveByPod(pod("app2", "uid-b"))).To(BeTrue())

		Expect(r.ReservedDevices()).To(ConsistOf("000a", "000c"))
		Expect(r.DeviceSets).To(Equal([][]string{{"000a"}, {"000c"}}))
	})

	It("does not release a predecessor's devices when a Pod reuses its name", func() {
		r := spyrev1alpha1.Reservation{}
		r.Upsert(pod("app1", "uid-a"), []string{"000a"}, now)

		Expect(r.RemoveByPod(pod("app1", "uid-b"))).To(BeFalse())
		Expect(r.ReservedDevices()).To(ConsistOf("000a"))
	})

	It("removes a reservation by its devices regardless of their order", func() {
		r := spyrev1alpha1.Reservation{}
		r.Upsert(pod("app1", "uid-a"), []string{"000a", "0009"}, now)
		r.Upsert(pod("app2", "uid-b"), []string{"000b"}, now)

		Expect(r.RemoveByDevices([]string{"0009", "000a"})).To(BeTrue())
		Expect(r.ReservedDevices()).To(ConsistOf("000b"))
		Expect(r.RemoveByDevices([]string{"ffff"})).To(BeFalse())
	})

	It("finds a reservation of a given size among the available devices", func() {
		r := spyrev1alpha1.Reservation{}
		r.Upsert(pod("app1", "uid-a"), []string{"000a", "0009"}, now)
		r.Upsert(pod("app2", "uid-b"), []string{"000b"}, now)

		e, ok := r.EntryForSize(1, []string{"000b", "000c"})
		Expect(ok).To(BeTrue())
		Expect(e.DeviceList).To(Equal([]string{"000b"}))

		// The two-device set is the right size but one of its devices is not on
		// offer, so it must not be handed out.
		_, ok = r.EntryForSize(2, []string{"000a", "000b"})
		Expect(ok).To(BeFalse())
	})
})

var _ = Describe("Reservation compatibility with older writers", func() {
	It("leaves an up-to-date reservation untouched", func() {
		r := spyrev1alpha1.Reservation{}
		r.Upsert(pod("app1", "uid-a"), []string{"000a"}, now)
		before := *r.DeepCopy()

		r.NormalizeFromLegacy()
		Expect(r).To(Equal(before))
	})

	It("adopts a reservation written only in the deprecated fields", func() {
		// What an old scheduler leaves behind: no entries at all.
		r := spyrev1alpha1.Reservation{
			PodsUnderScheduling: []spyrev1alpha1.Pod{
				{Name: "app1", Namespace: "myapp"},
				{Name: "app2", Namespace: "myapp"},
			},
			DeviceSets: [][]string{{"000a"}, {"000b"}},
		}
		r.NormalizeFromLegacy()

		Expect(r.Entries).To(HaveLen(2))
		Expect(r.ReservedDevices()).To(ConsistOf("000a", "000b"))
		Expect(r.Entries[0].Pod.Name).To(Equal("app1"))
		Expect(r.Entries[1].Pod.Name).To(Equal("app2"))
		// Adopted entries have no timestamp, so they are releasable at once
		// rather than pinning devices forever.
		Expect(r.Entries[0].ReservedAt).To(BeNil())
	})

	It("drops an entry whose devices an older writer retired", func() {
		r := spyrev1alpha1.Reservation{}
		r.Upsert(pod("app1", "uid-a"), []string{"000a"}, now)
		r.Upsert(pod("app2", "uid-b"), []string{"000b"}, now)

		// An old device plugin turned app1's reservation into an allocation and
		// rewrote only the deprecated fields.
		r.PodsUnderScheduling = []spyrev1alpha1.Pod{{Name: "app2", Namespace: "myapp", UID: k8stypes.UID("uid-b")}}
		r.DeviceSets = [][]string{{"000b"}}

		r.NormalizeFromLegacy()

		Expect(r.Entries).To(HaveLen(1))
		Expect(r.Entries[0].Pod).To(Equal(pod("app2", "uid-b")))
		Expect(r.ReservedDevices()).To(ConsistOf("000b"))
	})

	It("adopts a reservation an older writer added alongside existing entries", func() {
		r := spyrev1alpha1.Reservation{}
		r.Upsert(pod("app1", "uid-a"), []string{"000a"}, now)

		// An old scheduler appended a reservation of its own.
		r.PodsUnderScheduling = append(r.PodsUnderScheduling, spyrev1alpha1.Pod{Name: "app9", Namespace: "myapp"})
		r.DeviceSets = append(r.DeviceSets, []string{"0099"})

		r.NormalizeFromLegacy()

		Expect(r.Entries).To(HaveLen(2))
		Expect(r.ReservedDevices()).To(ConsistOf("000a", "0099"))
		Expect(r.Entries[0].Pod).To(Equal(pod("app1", "uid-a")))
		Expect(r.Entries[1].Pod.Name).To(Equal("app9"))
	})

	It("keeps devices reserved even when no Pod is left to own them", func() {
		// A device set with nobody claiming it still has to block the device -
		// forgetting it is what allows a second Pod onto the same card.
		r := spyrev1alpha1.Reservation{
			DeviceSets: [][]string{{"000a"}},
		}
		r.NormalizeFromLegacy()

		Expect(r.Entries).To(HaveLen(1))
		Expect(r.Entries[0].Pod).To(Equal(spyrev1alpha1.Pod{}))
		Expect(r.ReservedDevices()).To(ConsistOf("000a"))
	})

	It("ignores a Pod an older writer left without devices", func() {
		r := spyrev1alpha1.Reservation{
			PodsUnderScheduling: []spyrev1alpha1.Pod{{Name: "app1", Namespace: "myapp"}},
		}
		r.NormalizeFromLegacy()

		Expect(r.Entries).To(BeEmpty())
		Expect(r.IsEmpty()).To(BeTrue())
	})

	It("follows the device sets when an older writer swapped a Pod's devices in place", func() {
		// Same Pod, same number of reservations - only the devices moved. The
		// counts still line up, so nothing but comparing the sets themselves
		// notices, and DeviceSets is the authority.
		r := spyrev1alpha1.Reservation{}
		r.Upsert(pod("app1", "uid-a"), []string{"000a"}, now)
		r.DeviceSets = [][]string{{"000b"}}

		r.NormalizeFromLegacy()

		Expect(r.ReservedDevices()).To(ConsistOf("000b"))
	})
})

var _ = Describe("Reservation lookup by Pod", func() {
	It("returns the entry a Pod holds, and reports when it holds none", func() {
		r := spyrev1alpha1.Reservation{}
		r.Upsert(pod("app1", "uid-a"), []string{"000a"}, now)

		entry, found := r.EntryForPod(pod("app1", "uid-a"))
		Expect(found).To(BeTrue())
		Expect(entry.DeviceList).To(ConsistOf("000a"))

		// Same name, different generation: not this Pod's reservation.
		_, found = r.EntryForPod(pod("app1", "uid-z"))
		Expect(found).To(BeFalse())
	})
})

var _ = Describe("SpyreNodeStateStatus reservation helpers", func() {
	It("creates the map on first reservation and drops a pool once it empties", func() {
		s := &spyrev1alpha1.SpyreNodeStateStatus{}
		s.ReserveDevices("spyre_pf", pod("app1", "uid-a"), []string{"000a"}, now)
		Expect(s.Reservations).To(HaveKey("spyre_pf"))

		Expect(s.ReleaseReservation(pod("app1", "uid-a"))).To(BeTrue())
		Expect(s.Reservations).NotTo(HaveKey("spyre_pf"))
	})

	It("releases a Pod's reservations from every pool it holds one in", func() {
		s := &spyrev1alpha1.SpyreNodeStateStatus{}
		s.ReserveDevices("spyre_pf", pod("app1", "uid-a"), []string{"000a"}, now)
		s.ReserveDevices("spyre_vf", pod("app1", "uid-a"), []string{"000a.1"}, now)
		s.ReserveDevices("spyre_pf", pod("app2", "uid-b"), []string{"000b"}, now)

		Expect(s.ReleaseReservation(pod("app1", "uid-a"))).To(BeTrue())
		Expect(s.ReservedDevices()).To(ConsistOf("000b"))
		Expect(s.ReleaseReservation(pod("app1", "uid-a"))).To(BeFalse())
	})

	It("reports reserved and allocated devices across all pools", func() {
		s := &spyrev1alpha1.SpyreNodeStateStatus{
			AllocationList: []spyrev1alpha1.Allocation{
				{DeviceList: []string{"000f"}, Pod: &spyrev1alpha1.Pod{Name: "old", Namespace: "myapp"}},
			},
		}
		s.ReserveDevices("spyre_pf", pod("app1", "uid-a"), []string{"000a"}, now)
		s.ReserveDevices("spyre_vf", pod("app2", "uid-b"), []string{"000b"}, now)

		Expect(s.ReservedDevices()).To(ConsistOf("000a", "000b"))
		Expect(s.AllocatedDevices()).To(ConsistOf("000f"))
	})

	It("does not create a pool for a reservation of no devices", func() {
		// A Pod that asks for zero devices reserves nothing, and an empty pool
		// would be a placeholder for later readers to have to skip over.
		s := &spyrev1alpha1.SpyreNodeStateStatus{}
		s.ReserveDevices("spyre_pf", pod("app1", "uid-a"), nil, now)

		Expect(s.Reservations).NotTo(HaveKey("spyre_pf"))
		Expect(s.ReservationEntries()).To(BeEmpty())
	})

	It("pairs every entry with the pool holding it", func() {
		s := &spyrev1alpha1.SpyreNodeStateStatus{}
		s.ReserveDevices("spyre_pf", pod("app1", "uid-a"), []string{"000a"}, now)
		s.ReserveDevices("spyre_vf", pod("app2", "uid-b"), []string{"000b.1"}, now)

		entries := s.ReservationEntries()
		Expect(entries).To(HaveLen(2))
		Expect(entries["spyre_pf"]).To(HaveLen(1))
		Expect(entries["spyre_pf"][0].Pod).To(Equal(pod("app1", "uid-a")))
		Expect(entries["spyre_vf"][0].DeviceList).To(ConsistOf("000b.1"))
	})

	It("normalizes every pool and removes the empty placeholders older schedulers left", func() {
		s := &spyrev1alpha1.SpyreNodeStateStatus{
			Reservations: map[string]spyrev1alpha1.Reservation{
				"spyre_pf": {
					PodsUnderScheduling: []spyrev1alpha1.Pod{{Name: "app1", Namespace: "myapp"}},
					DeviceSets:          [][]string{{"000a"}},
				},
				"spyre_vf": {},
			},
		}
		s.NormalizeReservations()

		Expect(s.Reservations).NotTo(HaveKey("spyre_vf"))
		Expect(s.Reservations["spyre_pf"].Entries).To(HaveLen(1))
		Expect(s.ReservedDevices()).To(ConsistOf("000a"))
	})
})
