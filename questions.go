package main

import (
	"fmt"
	"strings"
)

func buildQuestions(config ExamConfig) []Question {
	topics := config.Topics
	if len(topics) == 0 {
		topics = []string{"일상생활", "여가"}
	}
	topic := func(i int) string { return topics[i%len(topics)] }
	q := []Question{
		{1, "Introduction", "자기소개", "Let's begin the interview. Please tell me about yourself in detail.", "Warm-up and spontaneous self-introduction"},
		{2, "Description", topic(0), fmt.Sprintf("You selected %s in your survey. Please describe it in as much detail as possible.", englishTopic(topic(0))), "Describe a familiar subject"},
		{3, "Routine", topic(0), fmt.Sprintf("What do you usually do related to %s? Walk me through a typical experience from beginning to end.", englishTopic(topic(0))), "Explain routine with sequence"},
		{4, "Past experience", topic(0), fmt.Sprintf("Tell me about a memorable experience you had with %s. What happened, and why do you remember it?", englishTopic(topic(0))), "Narrate a past event"},
		{5, "Description", topic(1), fmt.Sprintf("Now let's talk about %s. Describe the place, people, or activities involved.", englishTopic(topic(1))), "Give vivid details"},
		{6, "Comparison", topic(1), fmt.Sprintf("How has %s changed compared with the past? Give specific examples.", englishTopic(topic(1))), "Compare past and present"},
		{7, "Problem", topic(1), fmt.Sprintf("Tell me about a problem you experienced while doing something related to %s. How did you solve it?", englishTopic(topic(1))), "Explain a problem and resolution"},
		{8, "Role-play", topic(0), fmt.Sprintf("Imagine you need information about %s. Call the place and ask three or four questions.", englishTopic(topic(0))), "Ask relevant questions"},
		{9, "Role-play", topic(0), "I'm sorry, but there is a problem with your reservation. Explain the problem and offer two or three alternatives.", "Handle an unexpected situation"},
		{10, "Role-play follow-up", topic(0), "Tell me about a real experience when a plan changed unexpectedly. What did you do?", "Connect role-play to personal experience"},
		{11, "Unexpected", "기술", "Many people use AI and mobile apps every day. How have these technologies changed the way people study or work?", "Give an opinion with examples"},
		{12, "Unexpected", "환경", "What environmental issue is important in your community? Explain its causes and possible solutions.", "Discuss a social issue"},
		{13, "Advanced comparison", topic(0), fmt.Sprintf("Compare two different ways people can enjoy %s. What are the advantages and disadvantages of each?", englishTopic(topic(0))), "Develop a balanced comparison"},
		{14, "Opinion", topic(1), fmt.Sprintf("Some people say spending time on %s is essential for a good life. Do you agree or disagree? Why?", englishTopic(topic(1))), "Support a clear opinion"},
		{15, "Closing challenge", topic(0), fmt.Sprintf("Choose the most interesting change you expect in %s over the next ten years. Explain your prediction in detail.", englishTopic(topic(0))), "Make and support a prediction"},
	}
	return adaptDifficulty(q, config.Difficulty)
}

func adaptDifficulty(questions []Question, difficulty string) []Question {
	if strings.EqualFold(difficulty, "Novice") || strings.EqualFold(difficulty, "IM1") {
		for i := range questions {
			questions[i].Text += " You may use simple sentences and concrete examples."
		}
	}
	if strings.EqualFold(difficulty, "AL") {
		for i := range questions {
			questions[i].Text += " Include multiple perspectives, specific evidence, and a clear conclusion."
		}
	}
	return questions
}

func englishTopic(topic string) string {
	m := map[string]string{
		"집": "your home", "가족": "your family", "직장": "your work",
		"학교": "school life", "영화": "movies", "음악": "music",
		"운동": "exercise", "여행": "travel", "카페": "cafes",
		"요리": "cooking", "게임": "games", "쇼핑": "shopping",
		"공원": "parks", "일상생활": "your daily life", "여가": "your free time",
	}
	if value, ok := m[topic]; ok {
		return value
	}
	return topic
}

