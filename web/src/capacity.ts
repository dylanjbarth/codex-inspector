export const capacityCountdown=(reset:string,now:number)=>{const seconds=Math.max(0,Math.ceil((new Date(reset).getTime()-now)/1000));if(seconds===0)return'Reset time passed';const hours=Math.floor(seconds/3600),minutes=Math.floor((seconds%3600)/60),rest=seconds%60;return`${hours}h ${minutes}m ${rest}s until recorded reset`}

export const compactCapacityCountdown=(reset:string,now:number)=>{
  const seconds=Math.max(0,Math.ceil((new Date(reset).getTime()-now)/1000))
  if(seconds===0)return'Reset time passed'
  const days=Math.floor(seconds/86400),hours=Math.floor((seconds%86400)/3600),minutes=Math.floor((seconds%3600)/60)
  if(days>0)return`Resets in ${days}d ${hours}h`
  if(hours>0)return`Resets in ${hours}h ${minutes}m`
  return`Resets in ${minutes}m`
}

export const observationAge=(observed:string,now:number)=>{
  const seconds=Math.max(0,Math.floor((now-new Date(observed).getTime())/1000))
  if(seconds<60)return`Observed ${seconds}s ago`
  const minutes=Math.floor(seconds/60)
  if(minutes<60)return`Observed ${minutes} min ago`
  const hours=Math.floor(minutes/60)
  if(hours<24)return`Observed ${hours}h ago`
  return`Observed ${Math.floor(hours/24)}d ago`
}
