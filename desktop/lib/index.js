'use strict';

// One import for the shell mechanics every AgentWorks-family desktop app
// shares. desktop/ requires this directly; desktop-sparkquill/ consumes it
// as a file: dependency so a fix here reaches both apps and both packaged
// builds. See docs/design/sparkquill_desktop_on_platform_plan.md, P1.
module.exports = {
  ...require('./loginEnv'),
  ...require('./boundedLog'),
  ...require('./health'),
  ...require('./externalNav'),
  ...require('./shutdown'),
  ...require('./servers'),
};
