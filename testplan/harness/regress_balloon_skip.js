const { chromium } = require('playwright');
(async () => {
  const b = await chromium.launch();
  const p = await b.newPage();
  p.on('dialog', d => d.accept().catch(()=>{}));
  await p.goto('http://localhost:8099/', { waitUntil: 'networkidle' });
  await p.addStyleTag({ content: '.modal-overlay{display:none !important;}' });
  await p.evaluate(() => showScreen('amortization'));
  const setBase = async () => p.evaluate(async () => {
    const setv=(id,v)=>{const e=document.getElementById(id); if(e)e.value=v;};
    setv('amz-amount','10000'); setv('amz-loanDate','01/01/2025'); setv('amz-rate','8');
    setv('amz-firstDate','02/01/2025'); setv('amz-nPeriods','360'); setv('amz-perYr','12');
    setv('amz-payment','733.76'); setv('amz-points','0');
    document.getElementById('set-basis').value='365/360';
    document.getElementById('set-exact').value='yes';
    document.getElementById('set-balloonIncl').value='yes';
    const tr=document.querySelectorAll('#amz-balloon-body tr')[0];
    tr.querySelector('[data-amz-balloon-field="date"]').value='02/01/2030';
    tr.querySelector('[data-amz-balloon-field="amount"]').value='';
    await calcAmortization();
  });
  await setBase();
  const s1 = await p.evaluate(()=>document.querySelector('#amz-balloon-body [data-amz-balloon-field="amount"]').value);
  // add skip and recalc (THE USER'S WORKFLOW)
  const s2 = await p.evaluate(async ()=>{ document.getElementById('amz-skipMonths').value='7-8'; await calcAmortization();
    return document.querySelector('#amz-balloon-body [data-amz-balloon-field="amount"]').value; });
  console.log('1) solve no-skip       :', s1);
  console.log('2) add skip 7-8, recalc:', s2, '  (DOS: $-107,865.54)');
  // Type-over test: solve fresh, then TYPE a fixed balloon, recalc -> must honor typed value
  await p.evaluate(async ()=>{ document.getElementById('amz-skipMonths').value=''; await calcAmortization(); });
  const s3 = await p.evaluate(async ()=>{
    const cell=document.querySelector('#amz-balloon-body [data-amz-balloon-field="amount"]');
    cell.value='5000'; cell.dispatchEvent(new Event('input',{bubbles:true}));
    await calcAmortization();
    return { cell: cell.value, green: cell.classList.contains('cell-output') };
  });
  console.log('3) type 5000 over solved, recalc:', JSON.stringify(s3), ' (should stay 5000, not green, treated as fixed)');
  await b.close();
})();
