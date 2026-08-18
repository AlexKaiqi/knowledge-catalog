import { existsSync, mkdirSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import path from "node:path";
import type { PreviewGeneration, Proposal, ValidationReport } from "./maintenance.ts";

export interface ControlState {
  readonly proposals: Readonly<Record<string, Proposal>>;
  readonly previews: Readonly<Record<string, PreviewGeneration>>;
  readonly validations: Readonly<Record<string, ValidationReport>>;
}

export const EMPTY_CONTROL_STATE: ControlState = {
  proposals: {},
  previews: {},
  validations: {},
};

export class FileControlState {
  constructor(private readonly file: string) {}

  load(): ControlState {
    if (!existsSync(this.file)) return EMPTY_CONTROL_STATE;
    const raw = JSON.parse(readFileSync(this.file, "utf8")) as Partial<ControlState>;
    return {
      proposals: raw.proposals ?? {},
      previews: raw.previews ?? {},
      validations: raw.validations ?? {},
    };
  }

  save(state: ControlState): void {
    mkdirSync(path.dirname(this.file), { recursive: true });
    const tmp = `${this.file}.tmp`;
    writeFileSync(tmp, `${JSON.stringify(state, null, 2)}\n`, "utf8");
    renameSync(tmp, this.file);
  }
}
