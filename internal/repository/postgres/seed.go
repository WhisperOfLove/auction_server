package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func SeedUsers(ctx context.Context, pool *pgxpool.Pool) error {
	// Remove stale rows with same username (case-insensitive) so credentials always match seed.
	_, err := pool.Exec(ctx, `
DELETE FROM users
WHERE LOWER(TRIM(username)) IN (LOWER('Ehsan'), LOWER('Akbar'));

INSERT INTO users (
  id, username, password, display_name, gender, account_type, phone
) VALUES
  ('user-behnam', 'Behnam', 'ben', 'بهنام', 'male', 'شخصی', '09361207235'),
  ('user-elnaz', 'Elnaz', 'eli', 'الناز', 'female', 'شخصی', '09199928263'),
  ('user-ehsan', 'Ehsan', 'ehsan', 'احسان مانیان', 'male', 'شخصی', '09124063806'),
  ('user-akbar', 'Akbar', 'akbar', 'اکبر امجدی', 'male', 'شخصی', '09380842692')
ON CONFLICT (id) DO UPDATE SET
  username = EXCLUDED.username,
  password = EXCLUDED.password,
  display_name = EXCLUDED.display_name,
  gender = EXCLUDED.gender,
  account_type = EXCLUDED.account_type,
  phone = EXCLUDED.phone`)
	return err
}
