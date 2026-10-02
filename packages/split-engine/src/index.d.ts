export const engineVersion: string;
export interface Member { id: number; cond_tags?: string[] }
export interface Group { conds?: string[]; mode: 'weight' | 'exclude'; weight?: number }
export interface Rule { item_tag: string; groups: Group[]; rest?: Group | null }
export interface Detail {
  id?: number;
  amount: number;
  item_tag?: string;
  manual_member_ids?: number[] | null;
  custom_amounts?: Record<string, number>;
  /** The item's payer, who absorbs the detail when no member shares it. */
  payer_id?: number;
}
export interface SplitInput {
  detail: Detail;
  members: Member[];
  rules?: Rule[];
  split_order?: number[];
}
export interface Trace {
  /** e.g. `weighted`, `excluded`, `rest`, `custom`, `no-rule`, or `payer-absorbs` when nobody else shares the detail. */
  kind: string;
  rule_item_tag?: string;
  cond_set_index?: number;
  hit_cond_tags?: string[];
  weight?: number;
  total_weight?: number;
  unit_price?: number;
  remainder_bonus: number;
  value?: number;
}
export interface Share { member_id: number; amount: number; trace: Trace }
export interface SplitResult {
  shares: Share[];
  excluded: Share[];
  total_weight: number;
  unit_price: number;
  /** `no-participant` only when nobody shares the detail and `payer_id` is not a member. */
  validity: 'ok' | 'no-participant' | 'custom-mismatch' | 'custom-overflow';
  diff?: number;
}
export interface InitOptions {
  /** Override for frameworks that require explicit static-asset URLs. */
  wasmURL?: string | URL;
  /** For tests, Node, or applications that load the binary themselves. */
  wasmBytes?: ArrayBuffer | Uint8Array;
}
export type RuleIssueCode =
  | 'invalid-groups'
  | 'invalid-rest'
  | 'empty-cond-set'
  | 'duplicate-cond-set'
  | 'unknown-cond'
  | 'invalid-mode'
  | 'invalid-weight';
export interface ValidateRuleInput {
  /** Condition sets in priority order; the first match decides a member's weight. */
  groups: Group[];
  /** Fallback for members no group matched. Omitted means weight 1. */
  rest?: Group | null;
  /** The event's condition tag catalog; a rule may only reference these. */
  cond_tags: string[];
}
export type ValidateRuleResult =
  | { ok: true; groups: Group[]; rest: Group }
  | { ok: false; code: RuleIssueCode; detail: string };
export function initEngine(options?: InitOptions): Promise<void>;
export function splitDetail(input: SplitInput): SplitResult;
export function validateRule(input: ValidateRuleInput): ValidateRuleResult;
