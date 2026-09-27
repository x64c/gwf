package passwd

// Argon2idConf is the Argon2id parameters an app chooses — no defaults: passwd.NewArgon2id
// refuses a conf below the floor defined on passwd.Argon2id.
//
//	m := MemoryKiB    memory, KiB           t := Iterations  passes over the memory
//	p := Parallelism  lanes (threads)       SaltLen, KeyLen  bytes of salt and of hash
type Argon2idConf struct {
	MemoryKiB   uint32 `json:"memory_kib"`
	Iterations  uint32 `json:"iterations"`
	Parallelism uint8  `json:"parallelism"`
	SaltLen     uint32 `json:"salt_len"`
	KeyLen      uint32 `json:"key_len"`
}
