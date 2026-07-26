package framework

import "log"

func (c *Core) ResourceCleanUp() {
	log.Println("[INFO] App Resource Cleaning Up...")
	// Clean up DB clients ----
	// ToDo: factor out this
	for name, kvdbClient := range c.KVDBClients {
		log.Printf("[INFO] Closing KVDB client %q", name)
		if err := kvdbClient.Close(); err != nil {
			log.Printf("[ERROR] Failed to close KVDB client %q: %v", name, err)
		}
	}
	for name, sqlDBClient := range c.SQLDBClients {
		log.Printf("[INFO] Closing SQL DB client %q", name)
		if err := sqlDBClient.Close(); err != nil {
			log.Printf("[ERROR] Failed to close SQL DB client %q: %v", name, err)
		} else {
			log.Printf("[INFO] SQL DB client %q closed", name)
		}
	}
	//----
	log.Println("[INFO] App Resource Cleanup Complete")
}
