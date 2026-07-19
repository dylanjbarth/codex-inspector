async page => {
  await page.getByRole('heading', { name: 'Token & Capacity' }).waitFor();
  if (await page.evaluate(() => location.hash) !== '') throw new Error('bootstrap fragment remained');
  await page.getByText('2,500', { exact: true }).first().waitFor();
  await page.getByText('User root: 2,000', { exact: true }).waitFor();
  await page.getByText('Descendants: 500', { exact: true }).waitFor();
  await page.getByText('Stale recorded observation', { exact: true }).waitFor();
  await page.getByRole('link', { name: '2,500 tokens' }).click();
  await page.getByRole('heading', { name: 'Context Inspector' }).waitFor();
  await page.getByText('Causal session map', { exact: true }).waitFor();
  await page.getByText('2,000', { exact: true }).first().waitFor();
  await page.getByText('2,500', { exact: true }).first().waitFor();
  await page.getByRole('button', { name: /Turn 1 Completed/ }).click();
  await page.getByText('11 source-backed events.', { exact: false }).waitFor();
  await page.getByRole('button', { name: /03 Message/ }).click();
  await page.getByText('UTF-8 source bytes rendered as inert text.', { exact: false }).waitFor();
  await page.getByText('Unavailable in demo', { exact: true }).last().waitFor();
  await page.getByRole('link', { name: 'Review effectiveness' }).click();
  await page.getByRole('button', { name: 'Preview Review' }).click();
  await page.getByRole('heading', { name: 'Review launch preview' }).waitFor();
  await page.getByText('Separate model boundary', { exact: true }).waitFor();
  await page.getByRole('button', { name: 'Confirm & start Review' }).waitFor();
  return { previewed: true, fragmentRemoved: true };
}
