package ai

// Classifier classifies input.
type Classifier interface {
	Classify(input string) (string, error)
}
