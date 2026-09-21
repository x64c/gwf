package jobsched

// Scope is how many times one firing of a job happens across the processes
// running an app: once in each of them, or once among all of them. An INSTANCE
// is one process running the app; the APP is every instance sharing its name
// against its shared store — what coord.Mode calls one distributed app.
//
// The job declares its own scope, because the answer belongs to the work and
// not to the deployment. Logging a machine's health is once per instance
// however many instances there are; billing a customer is once per app however
// many instances there are. Both jobs can live in one app, so no deployment
// fact can decide between them.
//
// The zero value is not a scope. A job carrying it has not answered the
// question, and nothing here may answer it on the job's behalf: guessing turns
// work meant to happen once into work that happens N times, or work meant to
// happen everywhere into work that happens in one place.
type Scope int

const (
	_ Scope = iota // undeclared

	// OncePerInstance fires the job in every instance. What a job about the
	// instance itself wants: its machine's health, its memory, its files.
	OncePerInstance

	// OncePerApp fires the job in one instance while the others stand down,
	// so one firing happens however many instances are running. What a job
	// about shared state wants: one row written, one mail sent, one report
	// built.
	OncePerApp
)

func (s Scope) String() string {
	switch s {
	case OncePerInstance:
		return "once per instance"
	case OncePerApp:
		return "once per app"
	default:
		return "undeclared"
	}
}
