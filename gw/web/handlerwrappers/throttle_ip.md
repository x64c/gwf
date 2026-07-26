# IP Throttle Middleware

## WARNING

### False Positive (Shared IPs)

- Many users can share a single public IP
    - Mobile carriers often NAT millions of phones behind a handful of public IPs.
    - Corporate networks, hotels, schools, coffee shops
      → all devices exit through one IP.

- If you throttle "too many requests from IP X,"
  you might block a whole group of innocent users
  just because one of them was busy.

### Dynamic IPs

- Some ISPs frequently reassign IP addresses
- A user who exceeded limits might disconnect/reconnect
  and get a "clean" IP, bypassing throttling.
- Conversely, a new innocent user might inherit a "punished" IP
  and be throttled unfairly.

### IPv6 Explosion

- With IPv6, every device can have multiple unique addresses
- Malicious clients can rotate IPv6 addresses cheaply ("IP hopping")
  and bypass throttling rules that only track per-IP.
