Handle character work for this production — designing a new one, reviewing
what exists, or revising a character already in use. Figure out which of
these I mean from what follows, and if it's genuinely ambiguous, ask.

{{context}}

## If no characters exist yet, or I'm asking to add one

Walk me through designing it, following `video-cinematography`'s character
rules rather than skipping to a generation call:

1. Establish what makes this character recognizable — 3-6 specific,
   disambiguating visual attributes (not "a woman in her 30s", but the exact
   details that would let someone tell her apart from any other character in
   this production). This becomes the one phrase reused verbatim in every
   later shot, so get it concrete now rather than refining it after shots
   already exist.
2. Tell me which model and provider you're proposing to commit this
   character's whole arc to, and why, per `video-model-selection` — this is
   a real decision, not a default, and switching later is expensive.
3. Write the spec and generate the reference image, then call
   `show_character` and wait for me to approve it before generating any
   shot that uses it. An unapproved face propagates through the entire
   piece — this is the cheapest point in the whole production to catch it
   wrong.

## If characters already exist and I'm asking to see them

Call `show_character` again for each one so they're current in the
Production panel, and tell me for each: which model and provider its arc is
committed to, and — this is usually the more useful part — everything about
how it's actually being used: which shots reference it, whether any of
those shots are already generated, and whether its spec or reference image
has drifted from what those shots were conditioned on.

## If I'm asking to change an existing character

This is the expensive path, so be explicit about the cost before doing it.
Per `video-cinematography`, switching a character's model or provider
mid-arc is a last resort requiring my explicit sign-off — tell me how many
already-generated shots used the old version and would need to be
regenerated to match, not just that a change is possible. If I only want to
adjust the written spec (not the reference image or the model), that's
cheaper — say so, and say exactly which existing shots would then be out of
sync with the updated spec. Regenerate the reference image only after I've
confirmed I want the change, then `show_character` the result before it's
used in anything new.

## Either way

Treat this as a genuine conversation about the character, not a one-shot
lookup — if I ask a follow-up ("why that model", "what if her outfit were
different"), answer it against the actual production state rather than
generically.
