import { spawnClaude } from "./claude";
import { serializeOpenAIMessages } from "./serialize";

export async function handleOpenAIResponses(req: Request): Promise<Response> {
  let body: any = await req.json();
  let messages: Array<{role: string; content: string}> = [];
  if (typeof body.input === "string") messages = [{ role: "user", content: body.input }];
  else if (Array.isArray(body.input)) messages = body.input.map((m: any) => ({role: m.role, content: typeof m.content === "string" ? m.content : JSON.stringify(m.content)}));
  const { prompt, system } = serializeOpenAIMessages(messages);
  const finalSystem = body.instructions || system || undefined;
  const model = body.model || "claude-sonnet-5";
  const id = "resp_" + crypto.randomUUID().slice(0, 24);

  if (body.stream) {
    const encoder = new TextEncoder();
    let fullText = "";
    let sentCreated = false;
    const stream = new ReadableStream({
      async start(controller) {
        const send = (data: object) => controller.enqueue(encoder.encode("data: " + JSON.stringify(data) + "\n\n"));
        for await (const event of spawnClaude({ prompt, model, system: finalSystem })) {
          if (event.type !== "stream_event" || !("event" in event)) continue;
          const sse = (event as any).event as {type:string; delta?:{type:string; text?:string}};
          if (sse.type === "content_block_delta" && sse.delta?.type === "text_delta" && sse.delta.text) {
            if (!sentCreated) { send({type:"response.created",response:{id,object:"response",model,output:[{id:id+"_msg",type:"message",role:"assistant",content:[]}]}}); sentCreated=true; }
            fullText += sse.delta.text;
            send({type:"response.output_text.delta",item_id:id+"_msg",output_index:0,content_index:0,delta:sse.delta.text});
          }
        }
        send({type:"response.completed",response:{id,object:"response",model,output:[{id:id+"_msg",type:"message",role:"assistant",content:[{type:"output_text",text:fullText}]}]}});
        controller.close();
      }
    });
    return new Response(stream, {headers:{"Content-Type":"text/event-stream","Cache-Control":"no-cache"}});
  } else {
    let fullText = "";
    for await (const event of spawnClaude({ prompt, model, system: finalSystem })) {
      if (event.type !== "stream_event" || !("event" in event)) continue;
      const sse = (event as any).event as {type:string; delta?:{type:string; text?:string}};
      if (sse.type === "content_block_delta" && sse.delta?.type === "text_delta" && sse.delta.text) fullText += sse.delta.text;
    }
    return Response.json({id,object:"response",model,output:[{id:id+"_msg",type:"message",role:"assistant",content:[{type:"output_text",text:fullText}]}],usage:{input_tokens:0,output_tokens:0}});
  }
}
