// Package video is the Video Producer: give it a topic and it writes a
// narrated script, then has the workstation draw, animate, voice and edit it
// into an MP4 on the board task.
//
// The script is written here with the platform LLM. Everything with pixels or
// sound runs on the workstation image service (image-service/app/video.py),
// because the models need Apple's GPU — see docs/video-generation.md.
package video

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

// Runner is the launch surface satisfied by *Service.
type Runner interface {
	Start(ctx context.Context, req StartRequest) (uuid.UUID, error)
}

// Voices offered in the form. The workstation runs Kokoro; a voice's first
// letter is its language (a: US English, b: UK English, h: Hindi).
var voices = []model.ClarifyOption{
	{Label: "Heart — warm, US female", Value: "af_heart"},
	{Label: "Michael — calm, US male", Value: "am_michael"},
	{Label: "Bella — bright, US female", Value: "af_bella"},
	{Label: "Fenrir — deep, US male", Value: "am_fenrir"},
	{Label: "Emma — UK female", Value: "bf_emma"},
	{Label: "George — UK male", Value: "bm_george"},
	{Label: "Alpha — Hindi female", Value: "hf_alpha"},
	{Label: "Omega — Hindi male", Value: "hm_omega"},
}

const defaultVoice = "af_heart"

// Module is the Video Producer specialist.
//
// All specialists are wired this way: own package, then one Register call.
func Module(run Runner) agentmodule.Module {
	return agentmodule.Module{
		Builtin:  model.BuiltinVideoProducer,
		Label:    model.AgentNameVideo,
		Icon:     "clapperboard",
		Hint:     "Give it a topic: it writes a narrated script, then draws, animates, voices and edits it into an MP4 on the board task.",
		ChatHint: "To make a video, call agent_execute on the Video Producer with the topic; add length, format (landscape or portrait), voice and style when the user gives them. It runs for minutes to an hour and posts the finished video on the board task.",
		Schema:   schema(),
		Seed:     Seed,
		Launch:   launch(run),
		AbsorbPrompt: func(prompt string, vals map[string]string) {
			if strings.TrimSpace(vals["topic"]) == "" {
				vals["topic"] = strings.TrimSpace(prompt)
			}
		},
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin: model.BuiltinVideoProducer,
		Hint:    "A topic in, a narrated video out. Stills: ~15 min per minute of video; animated: ~2 h per minute.",
		Fields: []agentschema.Field{
			{Key: "topic", Label: "What is the video about?", Type: "textarea", Required: true, MinLength: 3,
				Placeholder: "Why lighthouses still matter in the age of GPS",
				Question:    "What should the video be about?"},
			{Key: "length", Label: "Length", Type: "select", Default: "60",
				Options: []model.ClarifyOption{
					{Label: "30 seconds", Value: "30"},
					{Label: "1 minute", Value: "60"},
					{Label: "90 seconds", Value: "90"},
					{Label: "2 minutes", Value: "120"},
					{Label: "3 minutes", Value: "180"},
				}},
			{Key: "format", Label: "Format", Type: "select", Default: "landscape",
				Options: []model.ClarifyOption{
					{Label: "Landscape 16:9 (YouTube, web)", Value: "landscape"},
					{Label: "Portrait 9:16 (Shorts, Reels, TikTok)", Value: "portrait"},
				}},
			{Key: "motion", Label: "Pictures", Type: "select", Default: "animated",
				Options: []model.ClarifyOption{
					{Label: "Animated (slow: ~12 min per shot, a 1-minute video ≈ 2 h)", Value: "animated"},
					{Label: "Still images with camera moves (fast: a 1-minute video ≈ 15 min)", Value: "stills"},
				}},
			{Key: "voice", Label: "Narrator voice", Type: "select", Default: defaultVoice, Options: voices},
			{Key: "style", Label: "Visual style (optional)", Type: "text",
				Placeholder: "cinematic documentary, warm golden-hour light",
				Help:        "Applied to every shot so the video looks consistent"},
			{Key: "captions", Label: "Captions", Type: "select", Default: "yes",
				Options: []model.ClarifyOption{
					{Label: "Burn in captions", Value: "yes"},
					{Label: "No captions", Value: "no"},
				}},
			{Key: "notes", Label: "Notes (optional)", Type: "textarea",
				Placeholder: "Audience, tone, points that must be covered, a call to action at the end…"},
		},
		TitleRules: []agentschema.TitleRule{
			{Prefix: "Video: ", FromKey: "topic", Truncate: 80, Fallback: "new video"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "length", Prefix: "Length (s): "},
			{Key: "format", Prefix: "Format: "},
			{Key: "motion", Prefix: "Pictures: "},
			{Key: "voice", Prefix: "Voice: "},
			{Key: "style", Prefix: "Style: "},
			{Key: "notes"},
		},
	}
}

// BriefFromValues resolves form values, defaulting anything missing or odd.
func BriefFromValues(v map[string]string) Brief {
	seconds, err := strconv.Atoi(strings.TrimSpace(v["length"]))
	if err != nil || seconds <= 0 {
		seconds = 60
	}
	seconds = min(max(seconds, 15), 180)

	format := "landscape"
	if strings.TrimSpace(v["format"]) == "portrait" {
		format = "portrait"
	}
	voice := strings.TrimSpace(v["voice"])
	known := false
	for _, o := range voices {
		known = known || o.Value == voice
	}
	if !known {
		voice = defaultVoice
	}
	return Brief{
		Topic:    strings.TrimSpace(v["topic"]),
		Seconds:  seconds,
		Format:   format,
		Voice:    voice,
		Style:    strings.TrimSpace(v["style"]),
		Motion:   strings.TrimSpace(v["motion"]) != "stills",
		Captions: strings.TrimSpace(v["captions"]) != "no",
		Notes:    strings.TrimSpace(v["notes"]),
	}
}

// Seed is the built-in Video Producer.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Turns a topic into a narrated video: writes the script, draws and animates each shot, records the narration, burns in captions and edits it into an MP4 on the board task. Renders on the workstation GPU."
	prompt := "You are the Video Producer. You turn a topic into a short narrated video: a clear, accurate spoken script with a strong opening and a clean ending, and for each line one concrete picture that shows what is being said. You never invent facts, figures or quotes."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         model.AgentNameVideo,
		Role:         "Video Producer",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinVideoProducer},
	}
}

func launch(run Runner) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if run == nil {
			return nil, fmt.Errorf("the Video Producer is not configured")
		}
		if in.Agent == nil {
			return nil, fmt.Errorf("video launch needs an agent")
		}
		req := StartRequest{OrgID: in.OrgID, UserID: in.UserID, AgentID: in.Agent.ID, Brief: BriefFromValues(in.Values)}
		if in.Task != nil {
			id := in.Task.ID
			req.TaskID = &id
		}
		runID, err := run.Start(ctx, req)
		if err != nil {
			return nil, err
		}
		return &agentmodule.LaunchOutput{
			RunID:     &runID,
			Message:   "Video Producer started: writing the script",
			ExtraMeta: map[string]any{model.TaskMetaRunID: runID.String()},
		}, nil
	}
}
