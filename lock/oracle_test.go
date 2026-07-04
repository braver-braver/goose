package lock

import "testing"

func TestOracleUserLockID(t *testing.T) {
	tests := []struct {
		name   string
		lockID int64
	}{
		{"default lock id", DefaultLockID},
		{"zero", 0},
		{"max user lock id", oracleMaxUserLockID - 1},
		{"overflows user lock range", oracleMaxUserLockID},
		{"negative", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := oracleUserLockID(tt.lockID)
			if got < 0 || got >= oracleMaxUserLockID {
				t.Errorf("oracleUserLockID(%d) = %d, outside DBMS_LOCK user range [0, %d)",
					tt.lockID, got, int64(oracleMaxUserLockID))
			}
		})
	}
	// Ids already in range must be preserved so that user-specified lock ids keep their meaning.
	if got := oracleUserLockID(123456789); got != 123456789 {
		t.Errorf("in-range lock id changed: got %d, want 123456789", got)
	}
	// The mapping must be deterministic and distinct sources should map predictably.
	if got := oracleUserLockID(DefaultLockID); got != DefaultLockID%oracleMaxUserLockID {
		t.Errorf("DefaultLockID mapping: got %d, want %d", got, DefaultLockID%oracleMaxUserLockID)
	}
}
