package ai

// MockClassifier is a placeholder classifier.
type MockClassifier struct{}

// Classify returns a placeholder classification.
func (c MockClassifier) Classify(input string) (string, error) {
	return "unknown", nil
}
