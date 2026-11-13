package service

import (
	whatsappDomain "chatnip/internal/modules/whatsapp/domain"
	"testing"
	"time"
)

func TestSessionSerializer_StashedJobFIFO(t *testing.T) {
	worker := &SessionWorker{
		ID:            "sess_test",
		FastQueue:     make(chan SessionJob, 10),
		StandardQueue: make(chan SessionJob, 10),
		Quit:          make(chan struct{}),
	}
	serializer := &SessionSerializer{}

	// Enqueue jobs for different JIDs in StandardQueue
	job1 := SessionJob{
		Payload: whatsappDomain.SendMessagePayload{RemoteJID: "jid_A", Text: "Msg 1"},
	}
	job2 := SessionJob{
		Payload: whatsappDomain.SendMessagePayload{RemoteJID: "jid_B", Text: "Msg 2"},
	}
	job3 := SessionJob{
		Payload: whatsappDomain.SendMessagePayload{RemoteJID: "jid_A", Text: "Msg 3"},
	}

	worker.StandardQueue <- job2
	worker.StandardQueue <- job3

	// Batch collected with firstJob = job1
	batch := serializer.collectBatch(worker, job1)

	// Since job2 has jid_B != jid_A, batch should only have job1, and job2 should be in worker.stashedJob
	if len(batch) != 1 {
		t.Fatalf("expected batch of size 1, got %d", len(batch))
	}
	if worker.stashedJob == nil {
		t.Fatalf("expected stashedJob to not be nil")
	}
	if worker.stashedJob.Payload.RemoteJID != "jid_B" {
		t.Errorf("expected stashedJob to be jid_B, got %s", worker.stashedJob.Payload.RemoteJID)
	}

	// Now pickNextJob should return the stashed job (jid_B) first
	timer := time.NewTimer(time.Second)
	defer timer.Stop()

	nextJob, ok := serializer.pickNextJob(worker, timer)
	if !ok {
		t.Fatalf("expected pickNextJob to succeed")
	}
	if nextJob.Payload.RemoteJID != "jid_B" {
		t.Errorf("expected nextJob to be jid_B, got %s", nextJob.Payload.RemoteJID)
	}
	if worker.stashedJob != nil {
		t.Errorf("expected stashedJob to be cleared after pickNextJob")
	}

	// And the following job should be job3 (jid_A)
	nextJob2, ok := serializer.pickNextJob(worker, timer)
	if !ok {
		t.Fatalf("expected pickNextJob to succeed for job3")
	}
	if nextJob2.Payload.RemoteJID != "jid_A" || nextJob2.Payload.Text != "Msg 3" {
		t.Errorf("expected nextJob2 to be job3, got %s", nextJob2.Payload.Text)
	}
}
