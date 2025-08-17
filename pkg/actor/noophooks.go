package actor

type noopHooks struct{}

func (nh *noopHooks) AfterStart()       {}
func (nh *noopHooks) AfterStop()        {}
func (nh *noopHooks) BeforeStart(_ any) {}
func (nh *noopHooks) BeforeStop(_ any)  {}
func (nh *noopHooks) OnError(_ error)   {}
