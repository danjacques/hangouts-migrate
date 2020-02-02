package parse

type semaphore chan struct{}

func (s semaphore) Acquire() {
	s <- struct{}{}
}

func (s semaphore) Release() {
	<-s
}

func (s semaphore) Wait() {
	for i := 0; i < cap(s); i++ {
		s.Acquire()
	}
}
