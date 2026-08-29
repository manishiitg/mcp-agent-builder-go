## Workspace provider tools

Only two provider-backed workspace tools are active:

- **`generate_text_llm(user_message, tier)`** runs one configured text-model
  call. `tier` is `low`, `medium`, or `high`.
- **`search_web_llm(query, provider)`** runs a live hosted-MCP web search.
  `provider` is `parallel`, `exa`, or `firecrawl`; it does not accept a
  `model_id` and never routes through a native coding-agent search tool.

Image, video, audio, music, transcription, and image-reading provider tools
are deprecated and hidden. Do not call them, advertise their providers, or
collect media-provider credentials through workspace provider setup.

For ordinary chat/text provider credentials, use the existing provider setup
flow. A Pi sub-provider credential (including MiniMax) is allowed only when
explicitly configuring Pi for a text model; it is not a media-provider setup.
