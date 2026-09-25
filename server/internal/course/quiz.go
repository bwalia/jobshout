package course

import (
	"fmt"
	"strings"

	"github.com/jobshout/server/internal/model"
)

const (
	quizOptions      = 4
	quizMinQuestions = 3
	quizMaxQuestions = 8
)

type quizWire struct {
	Questions []quizQuestionWire `json:"questions"`
}

type quizQuestionWire struct {
	Question     looseString  `json:"question"`
	Options      looseStrings `json:"options"`
	CorrectIndex looseInt     `json:"correct_index"`
	Explanation  looseString  `json:"explanation"`
}

// validateQuiz converts the model's reply to a quiz, keeping only
// well-formed questions: exactly four distinct options, one correct index in
// range, and an explanation. It fails when too few survive to be a quiz.
func validateQuiz(w quizWire) (*model.CourseQuiz, error) {
	quiz := &model.CourseQuiz{}
	seen := map[string]bool{}
	for _, q := range w.Questions {
		text := strings.TrimSpace(string(q.Question))
		if text == "" || seen[strings.ToLower(text)] {
			continue
		}
		opts := make([]string, 0, len(q.Options))
		distinct := map[string]bool{}
		for _, o := range q.Options {
			o = strings.TrimSpace(o)
			if o == "" || distinct[strings.ToLower(o)] {
				continue
			}
			distinct[strings.ToLower(o)] = true
			opts = append(opts, o)
		}
		idx := int(q.CorrectIndex)
		if len(opts) != quizOptions || len(opts) != len(q.Options) || idx < 0 || idx >= quizOptions {
			continue
		}
		expl := strings.TrimSpace(string(q.Explanation))
		if expl == "" {
			continue
		}
		seen[strings.ToLower(text)] = true
		quiz.Questions = append(quiz.Questions, model.CourseQuizQuestion{
			Question:     text,
			Options:      opts,
			CorrectIndex: idx,
			Explanation:  expl,
		})
		if len(quiz.Questions) == quizMaxQuestions {
			break
		}
	}
	if len(quiz.Questions) < quizMinQuestions {
		return nil, fmt.Errorf("quiz: only %d valid questions (need %d)", len(quiz.Questions), quizMinQuestions)
	}
	return quiz, nil
}
