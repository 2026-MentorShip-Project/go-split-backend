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
}
export interface SplitInput {
  detail: Detail;
  members: Member[];
  rules?: Rule[];
  split_order?: number[];
}
export interface Trace {
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
  validity: 'ok' | 'no-participant' | 'custom-mismatch' | 'custom-overflow';
  diff?: number;
}
export interface InitOptions {
  /** Override for frameworks that require explicit static-asset URLs. */
  wasmURL?: string | URL;
  /** For tests, Node, or applications that load the binary themselves. */
  wasmBytes?: ArrayBuffer | Uint8Array;
}
export function initEngine(options?: InitOptions): Promise<void>;
export function splitDetail(input: SplitInput): SplitResult;
