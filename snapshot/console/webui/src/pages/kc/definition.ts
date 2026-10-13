import { SourceInput } from '../../lib/api/kc';
import { sourceInputs, sourceRow } from './sourceEditor';
import {Metadata, metadataValue} from '../../lib/api/kcMetadata';

export type Definition = { dataset: string; revision: number; sources: SourceInput[]; metadata?: Metadata };
function fields(value: unknown, allowed: string[], label: string): Record<string, unknown> {
    if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error(`${label}必须是 JSON 对象。`);
    for (const key of Object.keys(value)) if (!allowed.includes(key)) throw new Error(`${label}不支持字段 ${key}，不会忽略后发布。`);
    return value as Record<string, unknown>;
}
export function parseDefinition(raw: string, expectedDataset?: string): Definition {
    const input = fields(JSON.parse(raw), ['dataset', 'revision', 'sources', 'metadata'], 'Dataset 定义');
    if (typeof input.dataset !== 'string' || !input.dataset.trim()) throw new Error('请填写 Dataset 名称。');
    if (expectedDataset && input.dataset !== expectedDataset) throw new Error('已发布 Dataset 的名称不能修改。');
    if (typeof input.revision !== 'number' || !Number.isSafeInteger(input.revision) || input.revision < 1) throw new Error('版本号必须是正整数。');
    if (!Array.isArray(input.sources) || input.sources.length === 0) throw new Error('至少添加一个来源。');
    const sources = input.sources.map((value, index) => {
        const source = fields(value, ['repository', 'selector', 'commit', 'baseRev', 'path', 'subPath', 'file', 'target'], `来源 ${index + 1}`);
        for (const [key, value] of Object.entries(source)) if (typeof value !== 'string') throw new Error(`来源 ${index + 1} 的 ${key} 必须是字符串。`);
        if (typeof source.repository !== 'string' || !source.repository.trim()) throw new Error(`请选择来源 ${index + 1} 的仓库。`);
        if (typeof source.file === 'string' && !source.file.trim()) throw new Error(`来源 ${index + 1} 的文件路径不能为空。`);
        if (source.file && (source.path !== undefined || source.subPath !== undefined)) throw new Error(`来源 ${index + 1} 的文件和目录声明不能混用。`);
        if (!source.file && source.target !== undefined) throw new Error(`来源 ${index + 1} 的 target 只用于单文件。`);
        return source as SourceInput;
    });
    return {dataset: input.dataset.trim(), revision: input.revision, sources: sourceInputs(sources.map(sourceRow)),
        ...(input.metadata === undefined ? {} : {metadata: metadataValue(input.metadata)})};
}
