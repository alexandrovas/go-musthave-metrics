package repository

import (
	"errors"
	"net"
	"testing"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

func TestIsRetriableDBError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{
			"connection exception (class 08)",
			&pgconn.PgError{Code: pgerrcode.ConnectionException},
			true,
		},
		{
			"connection does not exist (class 08)",
			&pgconn.PgError{Code: pgerrcode.ConnectionDoesNotExist},
			true,
		},
		{
			"transaction rollback (class 40)",
			&pgconn.PgError{Code: pgerrcode.TransactionRollback},
			true,
		},
		{
			"unique violation (class 23) is not retriable",
			&pgconn.PgError{Code: pgerrcode.UniqueViolation},
			false,
		},
		{
			"network error",
			&net.OpError{Op: "dial", Err: errors.New("connection refused")},
			true,
		},
		{
			"generic non-network error",
			errors.New("boom"),
			false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isRetriableDBError(tc.err))
		})
	}
}
