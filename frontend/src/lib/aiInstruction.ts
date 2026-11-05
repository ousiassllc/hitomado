/** 「AI への指示」の最大文字数（Go の ai.MaxInstructionRunes と同じ。docs/api §6。#109）。 */
export const MAX_INSTRUCTION_CHARS = 2000

/** 指示の文字数。Go と同じく前後の空白を除いたコードポイント数で数える（空白だけは指示なし）。 */
export function instructionLength(text: string): number {
  return [...text.trim()].length
}
