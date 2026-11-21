package mysql

import "testing"

// TestQualifyWrite pins the verb-anchored schema qualification and the
// precondition: a statement whose layout the verb pattern no longer
// matches must fail loudly instead of silently leaving the bare table
// reference in place.
func TestQualifyWrite(t *testing.T) {
	t.Run("qualifies the table reference after the verb", func(t *testing.T) {
		got, err := qualifyWrite("INSERT INTO ", "INSERT INTO `t5_x` (`id`) VALUES (?)", "other", "t5_x")
		if err != nil {
			t.Fatalf("qualifyWrite() error: %v", err)
		}
		want := "INSERT INTO `other`.`t5_x` (`id`) VALUES (?)"
		if got != want {
			t.Errorf("qualifyWrite() = %q, want %q", got, want)
		}
	})

	t.Run("does not qualify later occurrences of the name", func(t *testing.T) {
		got, err := qualifyWrite("UPDATE ", "UPDATE `t5_x` SET `a` = ? WHERE `t5_x`.`id` = ?", "other", "t5_x")
		if err != nil {
			t.Fatalf("qualifyWrite() error: %v", err)
		}
		want := "UPDATE `other`.`t5_x` SET `a` = ? WHERE `t5_x`.`id` = ?"
		if got != want {
			t.Errorf("qualifyWrite() = %q, want %q", got, want)
		}
	})

	t.Run("missing verb pattern is an error, not a silent no-op", func(t *testing.T) {
		_, err := qualifyWrite("INSERT INTO ", "SELECT 1", "other", "t5_x")
		if err == nil {
			t.Fatal("qualifyWrite() with unmatched verb: expected error, got nil")
		}
	})
}
