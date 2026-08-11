// ITEM 0j TARGETED SEARCH — LARGE NEGATIVE SEGMENT BALANCES.
// The flip window is bestp in (halfpenny, accLimit*|accInit|], i.e. it requires
// |accInit| > 0.005/2e-8 = 250,000 AND a negative accInit. accInit at
// fancybisect.go:1912 is `bal`, the SEGMENT balance at a payment-only (AO6)
// adjustment. A negative one arises when the adjustment is sited PAST the
// retirement point (unreachedAdjPrepass). To make it LARGE and negative we need
// a big loan, a big hard payment, and an adjustment far past retirement.
const http=require('http');
function post(req){return new Promise(res=>{const b=Buffer.from(JSON.stringify(req));
 const r=http.request({hostname:'localhost',port:8098,path:'/api/amortization/calc',method:'POST',
  headers:{'Content-Type':'application/json','Content-Length':b.length},timeout:20000},x=>{let d='';x.on('data',c=>d+=c);x.on('end',()=>{try{res(JSON.parse(d))}catch(e){res({error:'badjson'})}})});
 r.on('timeout',()=>{r.destroy();res({error:'timeout'})});r.on('error',e=>res({error:String(e.message)}));r.write(b);r.end();});}
function mul(a){return function(){a|=0;a=a+0x6D2B79F5|0;let t=Math.imul(a^a>>>15,1|a);t=t+Math.imul(t^t>>>7,61|t)^t;return((t^t>>>14)>>>0)/4294967296;};}
const seed=parseInt(process.argv[2]||'490490',10), N=parseInt(process.argv[3]||'600',10);
const rnd=mul(seed);
const iso=(y,m,d)=>`${y}-${String(m).padStart(2,'0')}-${String(d).padStart(2,'0')}`;
(async()=>{
 let sent=0;
 for(let i=0;i<N;i++){
   const perYr=[1,2,4,12][Math.floor(rnd()*4)];
   const y=2020+Math.floor(rnd()*8), m=1+Math.floor(rnd()*12), d=1+Math.floor(rnd()*28);
   const years=5+Math.floor(rnd()*30);
   const n=years*perYr;
   // BIG amounts and DELIBERATELY OVERSIZED hard payments so the balance is
   // driven far past zero before the adjustment date.
   const amount=Math.round((500000+rnd()*9500000)*100)/100;
   const rate=Math.round((0.02+rnd()*0.16)*1e10)/1e10;
   const overpay=2+rnd()*40;                       // 2x .. 42x the fair payment
   const fair=amount*(rate/perYr)/(1-Math.pow(1+rate/perYr,-n))||amount/n;
   const payment=Math.round(fair*overpay*100)/100;
   // adjustment sited LATE — well past the retirement point
   const k=Math.max(2,Math.floor(n*(0.5+rnd()*0.49)));
   const months=Math.round(12/perYr)*k;
   const ay=y+Math.floor((m-1+months)/12), am=((m-1+months)%12)+1;
   if(ay>2085) continue;
   const req={amount,rate,nPeriods:n,perYr,loanDate:iso(y,m,d),basis:['360','365','365/360'][Math.floor(rnd()*3)],
     payment,points:0,
     adjustments:[{date:iso(ay,am,d),amount:Math.round(payment*(0.1+rnd()*2)*100)/100}]};
   if(rnd()<0.5) req.usaRule=true;
   if(rnd()<0.4) req.prepayments=[{startDate:iso(ay,am,d),nPmts:1+Math.floor(rnd()*6),perYr,amount:Math.round(rnd()*5000*100)/100}];
   await post(req); sent++;
 }
 console.log('requests sent:',sent);
})();
