export interface QuantityRule {
    min: number;
    max: number;
    step: number;
    aggregation: 'INDEPENDENT' | 'ADDITIVE';
}
export interface ServiceItem {
    title: string;
    description: string;
    iconKey?: string;
    imageMediaId?: string;
}
export interface ServiceProfile {
    schemaVersion: 1;
    templateKey?: string;
    serviceTarget: string;
    billing: {
        unitDefinition: string;
        quantity: QuantityRule;
        quantityExplanation: string;
        visitFeeMode: 'INCLUDED' | 'WAIVED' | 'NOT_APPLICABLE';
        feeDescription: string;
    };
    includedItems: ServiceItem[];
    excludedItems?: ServiceItem[];
    unsuitableConditions?: string[];
    purchaseNotices?: string[];
    acceptanceCriteria: string[];
    afterSales: {
        type: 'WARRANTY' | 'REWORK' | 'SUPPORT';
        summary: string;
        coverage: string;
        exclusions?: string[];
        period?: {
            value: number;
            unit: 'HOUR' | 'DAY' | 'MONTH';
            startDescription: string;
        };
        contactDescription: string;
    };
    demandCollection: {
        description: {
            mode: 'HIDDEN' | 'OPTIONAL' | 'REQUIRED';
            label: string;
            hint: string;
        };
        media: {
            mode: 'HIDDEN' | 'OPTIONAL' | 'REQUIRED';
            label: string;
            hint: string;
        };
    };
    enhancements?: {
        tags?: string[];
        facts?: {
            label: string;
            value: string;
        }[];
        resources?: {
            name: string;
            provider: 'PROVIDER' | 'CUSTOMER' | 'SHARED';
            description: string;
        }[];
        preparations?: string[];
        steps?: ServiceItem[];
        gallery?: {
            mediaId: string;
            caption: string;
        }[];
        comparisons?: {
            title: string;
            beforeMediaId: string;
            afterMediaId: string;
            description: string;
            effectNotice: string;
        }[];
        faqs?: {
            question: string;
            answer: string;
        }[];
    } | null;
}
export function validQuantity(profile: ServiceProfile, n: number): boolean { const q = profile.billing.quantity; return Number.isInteger(n) && n >= q.min && n <= q.max && (n - q.min) % q.step === 0; }
