export function shouldReloadForRevision(currentEpoch:string|null,currentRevision:number|null,data:Record<string,unknown>){
  const revision=Number(data.revision)
  const epoch=typeof data.datasetEpoch==='string'?data.datasetEpoch:''
  return data.fullRefreshRequired===true&&currentEpoch!=null&&epoch!==''&&epoch!==currentEpoch&&Number.isFinite(revision)&&revision>(currentRevision??0)
}
