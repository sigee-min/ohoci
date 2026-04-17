import { getCurrentLocale, translate } from '@/i18n';

export function formatDateTime(value) {
  if (!value) {
    return '—';
  }
  try {
    return new Date(value).toLocaleString(getCurrentLocale());
  } catch {
    return String(value);
  }
}

export function formatCurrency(value, currency = 'USD') {
  const amount = Number(value);
  if (!Number.isFinite(amount)) {
    return '—';
  }
  try {
    return new Intl.NumberFormat(getCurrentLocale(), {
      style: 'currency',
      currency: currency || 'USD',
      maximumFractionDigits: 2
    }).format(amount);
  } catch {
    return `${currency || 'USD'} ${amount.toFixed(2)}`;
  }
}

export function formatNumber(value, maximumFractionDigits = 2) {
  const amount = Number(value);
  if (!Number.isFinite(amount)) {
    return '—';
  }
  return amount.toLocaleString(getCurrentLocale(), { maximumFractionDigits });
}

export function compactValue(value, head = 14, tail = 10) {
  const normalized = String(value || '').trim();
  if (!normalized) {
    return '—';
  }
  if (normalized.length <= head + tail + 1) {
    return normalized;
  }
  return `${normalized.slice(0, head)}…${normalized.slice(-tail)}`;
}

export function formatStatusLabel(status) {
  const normalized = String(status || '').trim();
  if (!normalized) {
    return translate('common.unknown', {}, getCurrentLocale());
  }

  const translationKey = `formatter.status.${normalized.toLowerCase()}`;
  const translated = translate(translationKey, {}, getCurrentLocale());
  if (translated !== translationKey) {
    return translated;
  }

  return normalized
    .replace(/[_-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .replace(/\b\w/g, (char) => char.toUpperCase());
}

export function statusVariant(status) {
  const normalized = String(status || '').trim().toLowerCase();
  switch (normalized) {
    case 'failed':
    case 'error':
    case 'warn':
      return 'destructive';
    case 'running':
    case 'launching':
    case 'provisioning':
    case 'verifying':
    case 'creating_image':
    case 'queued':
    case 'in_progress':
    case 'active':
    case 'resource_fallback':
      return 'secondary';
    case 'completed':
    case 'tag_verified':
    case 'available':
    case 'promoted':
    case 'processed':
    case 'enabled':
    case 'ready':
      return 'default';
    case 'cancelled':
    case 'tag_only':
    case 'info':
    case 'blocked':
      return 'outline';
    default:
      return 'outline';
  }
}

export function summarizeOCIAuthMode(mode, t = null) {
  const translateMode = (key) => (typeof t === 'function' ? t(key) : translate(key, {}, getCurrentLocale()));
  switch (mode) {
    case 'api_key':
      return translateMode('formatter.ociMode.api_key');
    case 'instance_principal':
      return translateMode('formatter.ociMode.instance_principal');
    case 'fake':
      return translateMode('formatter.ociMode.fake');
    default:
      return translateMode('common.unknown');
  }
}

export function describeSubnet(subnetOcid, subnetById = {}, defaultSubnetId = '') {
  if (!subnetOcid) {
    const defaultSubnet = subnetById[defaultSubnetId];
    if (defaultSubnet) {
      return translate('formatter.defaultSubnet.withName', { name: defaultSubnet.displayName || defaultSubnet.id }, getCurrentLocale());
    }
    return translate('formatter.defaultSubnet', {}, getCurrentLocale());
  }
  const subnet = subnetById[subnetOcid];
  if (!subnet) {
    return subnetOcid;
  }
  return subnet.displayName || subnet.id;
}

export function subnetOptionLabel(item) {
  const flags = [];
  if (item.isRecommended) {
    flags.push(translate('formatter.subnet.recommended', {}, getCurrentLocale()));
  }
  if (item.isCurrentDefault) {
    flags.push(translate('formatter.subnet.default', {}, getCurrentLocale()));
  }
  if (item.prohibitPublicIpOnVnic) {
    flags.push(translate('formatter.subnet.private', {}, getCurrentLocale()));
  }
  if (item.hasDefaultRouteToNat) {
    flags.push(translate('formatter.subnet.nat', {}, getCurrentLocale()));
  }
  if (item.hasDefaultRouteToInternet) {
    flags.push(translate('formatter.subnet.internet', {}, getCurrentLocale()));
  }
  return `${item.displayName || item.id} · ${item.cidrBlock}${flags.length ? ` (${flags.join(', ')})` : ''}`;
}

export function catalogSubnetOptionLabel(item) {
  const parts = [item.displayName || item.name || item.id];
  if (item.availabilityDomain) {
    parts.push(item.availabilityDomain);
  }
  if (item.cidrBlock) {
    parts.push(item.cidrBlock);
  }
  return parts.filter(Boolean).join(' · ');
}

export function catalogImageOptionLabel(item) {
  const parts = [item.displayName || item.id];
  const operatingSystem = [item.operatingSystem, item.operatingSystemVersion].filter(Boolean).join(' ');
  if (operatingSystem) {
    parts.push(operatingSystem);
  }
  if (item.timeCreated) {
    parts.push(formatDateTime(item.timeCreated));
  }
  return parts.filter(Boolean).join(' · ');
}

export function shapeOptionLabel(item) {
  const parts = [item.shape];
  parts.push(translate(item.isFlexible ? 'formatter.shape.flexible' : 'formatter.shape.fixed', {}, getCurrentLocale()));
  if (item.defaultOcpu != null && item.defaultMemoryGb != null) {
    parts.push(
      translate('formatter.shape.ocpuMemory', { ocpu: item.defaultOcpu, memoryGb: item.defaultMemoryGb }, getCurrentLocale())
    );
  }
  return parts.filter(Boolean).join(' · ');
}
