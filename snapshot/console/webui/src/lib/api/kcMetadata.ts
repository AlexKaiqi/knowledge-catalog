export type Metadata = Record<string, string>;
export type MetadataRow = {key: string; value: string};

export function metadataValue(value: unknown): Metadata {
    if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('自定义元数据必须是 JSON 字符串键值对象。');
    if (Object.values(value).some(item => typeof item !== 'string')) throw new Error('自定义元数据的每个值必须是字符串。');
    return value as Metadata;
}

export const metadataRows = (value: Metadata = {}): MetadataRow[] => Object.entries(value).map(([key, value]) => ({key, value}));

export function metadataFromRows(rows: MetadataRow[]): Metadata {
    if (new Set(rows.map(row => row.key)).size !== rows.length) throw new Error('自定义元数据的键不能重复。');
    return Object.fromEntries(rows.map(row => [row.key, row.value]));
}
