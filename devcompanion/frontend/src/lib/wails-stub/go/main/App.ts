export async function LoadConfig() {
  return {}
}

export async function SaveConfig() {
  return undefined
}

export async function OnCharaClick() {
  return undefined
}

export async function AnswerQuestion(question: string) {
  return undefined
}

export async function InstallOllama() {
  return undefined
}

export async function CancelInstall() {
  return undefined
}

export async function DetectSetupStatus() {
  return { is_first_run: false, detected_backends: [], has_claude_key: false }
}

export async function CompleteSetup() {
  return undefined
}

export async function ExpandForOnboarding() {
  return undefined
}
