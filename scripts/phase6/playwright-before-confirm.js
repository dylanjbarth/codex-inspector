async page => {
  const step = async (name, action) => {
    try {
      await action();
    } catch (error) {
      throw new Error(`browser-before-confirm:${name}: ${error instanceof Error ? error.message : String(error)}`);
    }
  };
  await step('dashboard-heading', () => page.getByRole('heading', { name: 'Token & Capacity' }).waitFor());
  if (await page.evaluate(() => location.hash) !== '') throw new Error('bootstrap fragment remained');
  await step('recorded-total', () => page.getByText('2,500', { exact: true }).first().waitFor());
  await step('user-root-total', () => page.getByText('User root: 2,000', { exact: true }).waitFor());
  await step('descendant-total', () => page.getByText('Descendants: 500', { exact: true }).waitFor());
  await step('capacity-staleness', () => page.getByText('Stale recorded observation', { exact: true }).waitFor());
  await step('named-session', () => page.getByRole('link', { name: /Fake widget work/ }).click());
  await step('context-heading', () => page.getByRole('heading', { name: 'Context Inspector' }).waitFor());
  await step('causal-map', () => page.getByText('Causal session map', { exact: true }).waitFor());
  await step('direct-total', () => page.getByText('2,000', { exact: true }).first().waitFor());
  await step('inclusive-total', () => page.getByText('2,500', { exact: true }).first().waitFor());
  await step('turn-selection', () => page.getByRole('button', { name: /Turn 1 Completed/ }).click());
  await step('event-count', () => page.getByText('11 source-backed events.', { exact: false }).waitFor());
  await step('message-selection', () => page.getByRole('button', { name: /03 Message/ }).click());
  await step('formatted-source', () => page.getByText('Recorded user message.', { exact: true }).waitFor());
  await step('original-record-disclosure', () => page.getByText('View original record', { exact: true }).click());
  await step('source-rendering', () => page.getByText('UTF-8 source bytes rendered as inert text.', { exact: false }).waitFor());
  await step('unavailable-evidence', () => page.getByText('Unavailable in demo', { exact: true }).last().waitFor());
  await step('review-link', () => page.getByRole('link', { name: 'Review effectiveness' }).click());
  await step('review-preview-action', () => page.getByRole('button', { name: 'Preview Review' }).click());
  await step('review-preview-heading', () => page.getByRole('heading', { name: 'Review launch preview' }).waitFor());
  await step('model-boundary', () => page.getByText('Separate model boundary', { exact: true }).waitFor());
  await step('review-confirm-action', () => page.getByRole('button', { name: 'Confirm & start Review' }).waitFor());
  return { previewed: true, fragmentRemoved: true };
}
