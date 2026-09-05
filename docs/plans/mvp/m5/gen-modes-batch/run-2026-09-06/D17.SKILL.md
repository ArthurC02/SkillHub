---
name: podcast-publishing-pipeline
description: Use this skill when the task is to turn an audio recording into a cleaned, short-form podcast package by transcribing it, removing filler words, writing a brief intro, and preparing it for Spotify. It follows the flow shown in the diagram and is appropriate when the agent can work from provided audio/text but cannot actually upload to external services without access.
---

# Podcast publishing pipeline

Follow the process in order.

## 1) Upload or stage the recording in Google Drive
- If the audio file is already available in the workspace, treat it as the source recording.
- If the task requires Google Drive upload but no live Drive access is available, do not claim the upload was completed. Instead, prepare the file and note that the upload step must be done by a user or by an agent with Drive access.
- Confirm the recording is the correct source before continuing.

## 2) Transcribe with Whisper
- Run or request a Whisper transcription of the recording.
- Preserve speaker turns if available.
- Keep timestamps only if they are useful for later editing.
- If transcription quality is poor, re-check the audio source before editing the text.

## 3) Remove filler words and口頭禪
- Clean the transcript by removing filler words, repeated starts, and habitual verbal tics.
- Keep the speaker’s meaning, tone, and factual content intact.
- Do not over-edit into a different style; the goal is a natural, polished spoken script.
- If a phrase is ambiguous, prefer minimal edits and preserve meaning.

## 4) Write a 100-word self-introduction
- Draft a concise self-introduction of about 100 Chinese characters/words as requested by the source flow.
- Make it sound natural and suitable for a podcast or profile context.
- Keep it focused on identity, background, and a brief value statement.
- If the task context provides a specific person or show, tailor the introduction to that context.

## 5) Prepare for Spotify
- Package the cleaned transcript and the 100-word introduction as the deliverables.
- If Spotify publishing access is not available, state plainly that the final upload cannot be completed here.
- Provide any metadata that would be needed for publishing, such as title, description, and episode notes, if the task asks for them.
- Ensure the final materials are ready for a human or connected tool to upload to Spotify.

## Completion check
- Source recording identified
- Whisper transcript produced
- Filler words removed
- 100-word introduction written
- Spotify-ready package prepared

If any external platform action cannot be performed in the current environment, stop at the highest completed step and clearly report what remains to be done.
