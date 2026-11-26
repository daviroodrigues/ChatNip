package service

import (
	hsemDomain "chatnip/internal/modules/hsem/domain"
	hsemService "chatnip/internal/modules/hsem/service"
	whatsappDomain "chatnip/internal/modules/whatsapp/domain"
	"context"
	"errors"
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

	batch := serializer.collectBatch(worker, job1)

	if len(batch) != 1 {
		t.Fatalf("expected batch of size 1, got %d", len(batch))
	}
	if worker.stashedJob == nil {
		t.Fatalf("expected stashedJob to not be nil")
	}
	if worker.stashedJob.Payload.RemoteJID != "jid_B" {
		t.Errorf("expected stashedJob to be jid_B, got %s", worker.stashedJob.Payload.RemoteJID)
	}

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

	nextJob2, ok := serializer.pickNextJob(worker, timer)
	if !ok {
		t.Fatalf("expected pickNextJob to succeed for job3")
	}
	if nextJob2.Payload.RemoteJID != "jid_A" || nextJob2.Payload.Text != "Msg 3" {
		t.Errorf("expected nextJob2 to be job3, got %s", nextJob2.Payload.Text)
	}
}

func TestSessionGate_FastPreemptsStandard(t *testing.T) {
	gate := &SessionGate{}
	ctx := context.Background()

	if err := gate.AcquireFast(ctx); err != nil {
		t.Fatalf("AcquireFast failed: %v", err)
	}

	stdDone := make(chan struct{})
	go func() {
		if err := gate.AcquireStandard(ctx); err != nil {
			t.Errorf("AcquireStandard failed: %v", err)
			return
		}
		gate.Release()
		close(stdDone)
	}()

	select {
	case <-stdDone:
		t.Fatal("AcquireStandard should not proceed while fast holds the gate")
	case <-time.After(60 * time.Millisecond):
	}

	gate.Release()

	select {
	case <-stdDone:
	case <-time.After(time.Second):
		t.Fatal("AcquireStandard did not acquire after fast released")
	}
}

func TestSessionGate_FastTriggersStandardPreemption(t *testing.T) {
	gate := &SessionGate{}
	ctx := context.Background()

	if err := gate.AcquireStandard(ctx); err != nil {
		t.Fatalf("AcquireStandard failed: %v", err)
	}

	stepCtx, stepCancel := context.WithCancelCause(ctx)
	gate.RegisterStandardCancel(stepCancel)

	fastDone := make(chan error, 1)
	go func() {
		err := gate.AcquireFast(ctx)
		if err == nil {
			gate.Release()
		}
		fastDone <- err
	}()

	select {
	case <-stepCtx.Done():
		if !errors.Is(context.Cause(stepCtx), ErrPreempted) {
			t.Fatalf("expected ErrPreempted cause, got %v", context.Cause(stepCtx))
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected AcquireFast to trigger standard preemption")
	}

	gate.ClearStandardCancel()
	gate.Release()

	select {
	case err := <-fastDone:
		if err != nil {
			t.Fatalf("AcquireFast returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("AcquireFast did not complete after standard released")
	}
}

func TestComputeBatchMarkAsRead(t *testing.T) {
	batch := []SessionJob{
		{Payload: whatsappDomain.SendMessagePayload{QuotedMsgID: "msg_1", Text: "Oi"}},
		{Payload: whatsappDomain.SendMessagePayload{QuotedMsgID: "msg_2", Text: "Tudo bem por ai?"}},
		{Payload: whatsappDomain.SendMessagePayload{TargetMsgID: "msg_3", Text: "Me avisa quando puder."}},
	}
	latestID, overhead := computeBatchMarkAsRead(batch)
	if latestID != "msg_3" {
		t.Fatalf("expected latest message ID msg_3, got %q", latestID)
	}
	if overhead <= 0 {
		t.Fatalf("expected positive sublinear reading overhead, got %v", overhead)
	}
}

func TestAttenuateBurstPauses_ForcesMicroPauseAfterThreshold(t *testing.T) {
	svc := &WhatsAppService{
		hsemEngine: hsemService.NewHSEMEngine(hsemService.NewMathUtils()),
	}

	steps := []hsemDomain.ActionStep{
		{Type: hsemDomain.StepStateChange, Timestamp: 0, Metadata: "composing"},
		{Type: hsemDomain.StepStateChange, Timestamp: 8 * time.Second, Metadata: "paused"},
		{Type: hsemDomain.StepStateChange, Timestamp: 9 * time.Second, Metadata: "composing"},
		{Type: hsemDomain.StepStateChange, Timestamp: 16 * time.Second, Metadata: "paused"},
		{Type: hsemDomain.StepAction, Timestamp: 17 * time.Second, Metadata: "send_message"},
	}

	var accum time.Duration
	res := svc.attenuateBurstPauses(steps, &accum, 12*time.Second)

	pausedCount := 0
	for _, s := range res {
		if s.Metadata == "paused" {
			pausedCount++
		}
	}
	if pausedCount != 1 {
		t.Fatalf("expected exactly 1 forced micro-pause after crossing 12s composing, got %d (%+v)", pausedCount, res)
	}
}
