package state

// Migrate upgrades s in-place to the current schema version.
// It is idempotent: calling it on a v2 state returns s unchanged.
// Version 0 (absent "version" key in JSON, which unmarshals to 0)
// is treated identically to Version 1.
func Migrate(s *State) *State {
	if s.Version < 2 {
		s.Version = 2
	}
	return s
}
