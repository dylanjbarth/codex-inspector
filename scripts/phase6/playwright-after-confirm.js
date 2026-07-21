async page => {
  await page.getByRole('button', { name: 'Confirm & start Review' }).click();
  await page.getByRole('heading', { name: 'Synthetic Phase 6 browser report.' }).waitFor({ timeout: 15000 });
  await page.getByText('Complete', { exact: true }).waitFor();
  await page.getByText('1 findings', { exact: true }).waitFor();
  await page.getByRole('link', { name: 'Open cited event' }).click();
  await page.getByRole('heading', { name: 'Context Inspector' }).waitFor();
  await page.getByRole('link', { name: 'Return to Review' }).click();
  await page.getByRole('heading', { name: 'Synthetic Phase 6 browser report.' }).waitFor();

  const openTask = page.getByRole('link', { name: 'Open Codex task' });
  const taskHref = await openTask.getAttribute('href');
  if (!taskHref || !taskHref.startsWith('codex://threads/')) throw new Error('desktop task handoff missing');
  await openTask.evaluate(element => element.addEventListener('click', event => {
    event.preventDefault();
    window.__phase6OpenedTask = element.getAttribute('href');
  }, { once: true }));
  await openTask.click();
  const intercepted = await page.evaluate(() => window.__phase6OpenedTask);
  if (intercepted !== taskHref) throw new Error('desktop task handoff was not safely intercepted');

  const origin = await page.evaluate(() => location.origin);
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'], { origin });
  await page.getByRole('button', { name: 'Copy resume command' }).click();
  await page.getByText('Copied', { exact: true }).waitFor();
  const copied = await page.evaluate(() => navigator.clipboard.readText());
  if (!copied.startsWith('codex resume ') || !taskHref.endsWith(copied.slice('codex resume '.length))) {
    throw new Error('resume handoff mismatch');
  }
  return { reportRendered: true, citationReturned: true, handoffVerified: true };
}
