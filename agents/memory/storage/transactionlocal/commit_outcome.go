//go:build linux

package transactionlocal

// syncPublishedDirectory confirms that the newly published pointer is durable.
// The internal function override supports deterministic filesystem-failure tests;
// production instances always use the real filesystem operation.
func (engine *Engine) syncPublishedDirectory() error {
	if engine.commitSync != nil {
		return engine.commitSync(engine.root)
	}
	return syncDirectory(engine.root)
}
