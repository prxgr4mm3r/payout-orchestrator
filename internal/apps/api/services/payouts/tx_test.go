package payouts

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
)

type fakeBeginner struct {
	begin func(ctx context.Context) (pgx.Tx, error)
}

func (f fakeBeginner) Begin(ctx context.Context) (pgx.Tx, error) {
	return f.begin(ctx)
}

type fakeDBTX struct{}

func (fakeDBTX) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (fakeDBTX) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (fakeDBTX) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	return nil
}

type fakePGXTx struct {
	commitErr     error
	rollbackErr   error
	commitCalls   int
	rollbackCalls int
}

func (f *fakePGXTx) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("nested transactions are not used in tests")
}

func (f *fakePGXTx) Commit(context.Context) error {
	f.commitCalls++
	return f.commitErr
}

func (f *fakePGXTx) Rollback(context.Context) error {
	f.rollbackCalls++
	return f.rollbackErr
}

func (f *fakePGXTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, nil
}

func (f *fakePGXTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	return nil
}

func (f *fakePGXTx) LargeObjects() pgx.LargeObjects {
	return pgx.LargeObjects{}
}

func (f *fakePGXTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, nil
}

func (f *fakePGXTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (f *fakePGXTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (f *fakePGXTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return nil
}

func (f *fakePGXTx) Conn() *pgx.Conn {
	return nil
}

func TestDBTxRunnerCommitsOnSuccess(t *testing.T) {
	t.Parallel()

	tx := &fakePGXTx{}
	runner := NewDBTxRunner(fakeBeginner{
		begin: func(context.Context) (pgx.Tx, error) {
			return tx, nil
		},
	}, db.New(fakeDBTX{}))

	err := runner.WithinTx(context.Background(), func(store PayoutStore) error {
		if _, ok := store.(*db.Queries); !ok {
			t.Fatalf("expected transactional store to be *db.Queries, got %T", store)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("within tx: %v", err)
	}
	if tx.commitCalls != 1 {
		t.Fatalf("expected 1 commit call, got %d", tx.commitCalls)
	}
	if tx.rollbackCalls != 0 {
		t.Fatalf("expected 0 rollback calls, got %d", tx.rollbackCalls)
	}
}

func TestDBTxRunnerRollsBackOnHandlerError(t *testing.T) {
	t.Parallel()

	tx := &fakePGXTx{}
	runner := NewDBTxRunner(fakeBeginner{
		begin: func(context.Context) (pgx.Tx, error) {
			return tx, nil
		},
	}, db.New(fakeDBTX{}))
	expectedErr := errors.New("handler failed")

	err := runner.WithinTx(context.Background(), func(PayoutStore) error {
		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
	if tx.commitCalls != 0 {
		t.Fatalf("expected 0 commit calls, got %d", tx.commitCalls)
	}
	if tx.rollbackCalls != 1 {
		t.Fatalf("expected 1 rollback call, got %d", tx.rollbackCalls)
	}
}

func TestDBTxRunnerRollsBackOnCommitError(t *testing.T) {
	t.Parallel()

	tx := &fakePGXTx{commitErr: errors.New("commit failed")}
	runner := NewDBTxRunner(fakeBeginner{
		begin: func(context.Context) (pgx.Tx, error) {
			return tx, nil
		},
	}, db.New(fakeDBTX{}))

	err := runner.WithinTx(context.Background(), func(PayoutStore) error {
		return nil
	})
	if !errors.Is(err, tx.commitErr) {
		t.Fatalf("expected commit error %v, got %v", tx.commitErr, err)
	}
	if tx.commitCalls != 1 {
		t.Fatalf("expected 1 commit call, got %d", tx.commitCalls)
	}
	if tx.rollbackCalls != 1 {
		t.Fatalf("expected 1 rollback call after commit error, got %d", tx.rollbackCalls)
	}
}
