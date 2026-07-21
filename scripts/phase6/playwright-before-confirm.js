async page => {
  const step = async (name, action) => {
    try {
      await action();
    } catch (error) {
      throw new Error(`browser-before-confirm:${name}: ${error instanceof Error ? error.message : String(error)}`);
    }
  };
  const assertContrast = async (name, locator, minimum = 4.5) => {
    const result = await locator.evaluate(element => {
      const channels = value => value.match(/[\d.]+/g).slice(0, 3).map(Number);
      const luminance = value => {
        const normalized = channels(value).map(channel => channel / 255).map(channel => channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4);
        return 0.2126 * normalized[0] + 0.7152 * normalized[1] + 0.0722 * normalized[2];
      };
      const foreground = getComputedStyle(element).color;
      let background = 'rgba(0, 0, 0, 0)';
      for (let node = element; node; node = node.parentElement) {
        const candidate = getComputedStyle(node).backgroundColor;
        if (!candidate.endsWith(', 0)') && candidate !== 'transparent') {
          background = candidate;
          break;
        }
      }
      const foregroundLuminance = luminance(foreground);
      const backgroundLuminance = luminance(background);
      return { foreground, background, ratio: (Math.max(foregroundLuminance, backgroundLuminance) + 0.05) / (Math.min(foregroundLuminance, backgroundLuminance) + 0.05) };
    });
    if (result.ratio < minimum) throw new Error(`${name} contrast ${result.ratio.toFixed(2)}:1 (${result.foreground} on ${result.background}) is below ${minimum}:1`);
  };
  await step('dashboard-heading', () => page.getByRole('heading', { name: 'Token & Capacity' }).waitFor());
  if (await page.evaluate(() => location.hash) !== '') throw new Error('bootstrap fragment remained');
  await step('review-history-navigation', () => page.getByRole('link', { name: 'Reviews' }).click());
  await step('review-history-heading', () => page.getByRole('heading', { name: 'Evidence-linked guidance, kept local' }).waitFor());
  await step('review-history-heading-contrast', () => assertContrast('review history heading', page.getByRole('heading', { name: 'Evidence-linked guidance, kept local' })));
  await step('review-history-action-contrast', () => assertContrast('new review action', page.getByRole('button', { name: 'New review' })));
  await step('review-history-empty-contrast', () => assertContrast('review history empty copy', page.getByText('Create a plan, inspect exactly what Codex will receive, then explicitly confirm the launch.')));
  await page.setViewportSize({ width: 390, height: 844 });
  await step('review-history-mobile-overflow', async () => {
    const sizes = await page.evaluate(() => ({ viewport: innerWidth, document: document.documentElement.scrollWidth }));
    if (sizes.document > sizes.viewport) throw new Error(`review history overflows mobile viewport (${sizes.document}px > ${sizes.viewport}px)`);
  });
  await page.setViewportSize({ width: 1440, height: 900 });
  await step('dashboard-return', () => page.getByRole('link', { name: 'Dashboard' }).click());
  await step('dashboard-return-heading', () => page.getByRole('heading', { name: 'Token & Capacity' }).waitFor());
  await step('recorded-total', () => page.getByText('2,500', { exact: true }).first().waitFor());
  await step('user-root-total', () => page.getByText('User root: 2,000', { exact: true }).waitFor());
  await step('descendant-total', () => page.getByText('Descendants: 500', { exact: true }).waitFor());
  await step('capacity-staleness', () => page.getByText('Stale recorded observation', { exact: true }).waitFor());
  await step('capacity-bar-chart', async () => {
    const chart = page.getByRole('img', { name: 'Recorded capacity drawdown bar chart, with utilization percentage on the vertical axis' });
    await chart.waitFor();
    const bars = await chart.locator('.recharts-bar-rectangle').count();
    const legendItems = await chart.locator('.recharts-legend-item').count();
    const limitWindows = await page.locator('.capacity-series > li').count();
    if (bars < 2) throw new Error(`capacity bar chart rendered ${bars} bars`);
    if (legendItems !== limitWindows) throw new Error(`capacity bar chart rendered ${legendItems} legend items for ${limitWindows} limit windows`);
  });
  await page.setViewportSize({ width: 1100, height: 900 });
  await step('session-table-responsive-layout', async () => {
    const layout = await page.getByRole('link', { name: /Fake widget work/ }).evaluate(link => {
      const row = link.closest('[role="row"]');
      if (!row || row.children.length !== 4) throw new Error('session row structure unavailable');
      const project = row.children[1];
      const tokens = row.children[2];
      const range = document.createRange();
      range.selectNodeContents(project);
      const projectText = range.getBoundingClientRect();
      const tokenCell = tokens.getBoundingClientRect();
      return {
        documentWidth: document.documentElement.scrollWidth,
        viewportWidth: innerWidth,
        cellsOverlap: projectText.right > tokenCell.left && projectText.left < tokenCell.right && projectText.bottom > tokenCell.top && projectText.top < tokenCell.bottom,
      };
    });
    if (layout.documentWidth > layout.viewportWidth) throw new Error(`session table overflows viewport (${layout.documentWidth}px > ${layout.viewportWidth}px)`);
    if (layout.cellsOverlap) throw new Error('project text overlaps recorded tokens');
  });
  await page.setViewportSize({ width: 1440, height: 900 });
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
