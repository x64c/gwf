package namedlocks

import "sync"

// AcquireLocks registers the namedlocks into the lockStore (registry) to show they are already acquired by someone
// so more attempts to acquire the locks will be blocked
func AcquireLocks(lockStore *sync.Map, lockNames []string) ([]string, bool) {
	//sort.Strings(lockNames) // optional, prevents deadlocks if WAIT mode added
	var acquiredLockNames []string
	for _, lockName := range lockNames {
		_, lockedOut := lockStore.LoadOrStore(lockName, struct{}{})
		if lockedOut { // found a lock already acquired and registered by someone else
			// so, failed to acquire all the required locks.
			// release all locks acquired up to this point
			for _, k := range acquiredLockNames {
				lockStore.Delete(k)
			}
			return nil, false
		}
		acquiredLockNames = append(acquiredLockNames, lockName)
	}
	return acquiredLockNames, true
}

// ReleaseLocks deregisters the namedlocks by deleting the lock names from the lockStore
// Invoke this in a deferred call to guarantee to be called even if panic occurs.
func ReleaseLocks(lockStore *sync.Map, lockNames []string) {
	for _, lockName := range lockNames {
		lockStore.Delete(lockName)
	}
}
