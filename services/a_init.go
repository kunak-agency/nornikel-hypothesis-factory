package services

import (
	"hypothesis-factory/externalApi"
	"hypothesis-factory/repositories"
	"hypothesis-factory/services/feedback"
	"hypothesis-factory/services/hypothesisfactory"
	"hypothesis-factory/services/knowledgebase"
)

type Services struct {
	KnowledgeBase *knowledgebase.Service
	Pipeline      *hypothesisfactory.Service
	Feedback      *feedback.Service
}

func NewServices(repos *repositories.Repos, llm externalApi.LLMClient, pyworker *externalApi.PyworkerClient) *Services {
	return &Services{
		KnowledgeBase: knowledgebase.NewService(repos, pyworker),
		Pipeline:      hypothesisfactory.NewService(repos, llm, pyworker),
		Feedback:      feedback.NewService(repos),
	}
}
