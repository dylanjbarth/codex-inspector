package phase0

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// ActivateValidatedEpoch is the only supported application path from a
// building epoch to active. The caller switches the external catalog only
// after this transaction commits.
func ActivateValidatedEpoch(ctx context.Context, tx *sql.Tx, epochID, validatedAt, activatedAt, validationSHA256 string) error {
	if tx == nil {
		return errors.New("epoch activation requires a transaction")
	}
	if len(validationSHA256) != 64 || validationSHA256 != strings.ToLower(validationSHA256) {
		return errors.New("epoch activation requires a lowercase SHA-256 validation digest")
	}
	for _, character := range validationSHA256 {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return errors.New("epoch activation requires a lowercase SHA-256 validation digest")
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO epoch_validations(epoch_id,validated_at,validation_sha256) VALUES(?,?,?)`, epochID, validatedAt, validationSHA256); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE dataset_epochs SET state='active',activated_at=? WHERE id=? AND state='building'`, activatedAt, epochID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return errors.New("epoch activation requires exactly one building epoch")
	}
	return nil
}
