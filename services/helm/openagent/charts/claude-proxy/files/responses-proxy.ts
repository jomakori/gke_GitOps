// responses-proxy: translates /v1/responses → /v1/chat/completions on claude-pipe
const PORT = parseInt(process.env.PORT ?? '4524');
const UPSTREAM = process.env.UPSTREAM ?? 'http://127.0.0.1:4523';

Bun.serve({
  port: PORT,
  hostname: '0.0.0.0',
  async fetch(req) {
    const url = new URL(req.url);
    
    if (url.pathname !== '/v1/responses' || req.method !== 'POST') {
      const upstreamUrl = UPSTREAM + url.pathname + url.search;
      return fetch(upstreamUrl, {
        method: req.method,
        headers: req.headers,
        body: req.body,
      });
    }

    const body = await req.json() as any;
    const input = typeof body.input === 'string' ? body.input : JSON.stringify(body.input);
    
    const chatReq = {
      model: body.model || 'claude-sonnet-5',
      messages: [
        ...(body.instructions ? [{ role: 'system', content: body.instructions }] : []),
        { role: 'user', content: input }
      ],
      stream: body.stream ?? false,
    };

    console.log(`[responses-proxy] → /v1/chat/completions model=${chatReq.model} stream=${chatReq.stream}`);

    const chatResp = await fetch(`${UPSTREAM}/v1/chat/completions`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(chatReq),
    });

    if (!chatResp.ok) {
      const errText = await chatResp.text();
      console.error(`Upstream error: ${chatResp.status} ${errText.slice(0,200)}`);
      return new Response(errText, { status: chatResp.status, headers: chatResp.headers });
    }

    if (body.stream) {
      const encoder = new TextEncoder();
      const id = `resp_${crypto.randomUUID().slice(0, 24)}`;
      const reader = chatResp.body!.getReader();
      const decoder = new TextDecoder();
      let buf = '';
      let fullText = '';
      let sentCreated = false;

      const stream = new ReadableStream({
        async pull(controller) {
          const { done, value } = await reader.read();
          if (done) {
            controller.close();
            return;
          }
          buf += decoder.decode(value, { stream: true });
          const lines = buf.split('\n');
          buf = lines.pop() ?? '';

          for (const line of lines) {
            if (!line.startsWith('data: ')) continue;
            const data = line.slice(6);
            if (data === '[DONE]') {
              const done = JSON.stringify({
                type: 'response.completed',
                response: {
                  id, object: 'response', model: chatReq.model,
                  output: [{ id: `${id}_msg`, type: 'message', role: 'assistant',
                    content: [{ type: 'output_text', text: fullText }] }],
                }
              });
              controller.enqueue(encoder.encode(`data: ${done}\n\n`));
              continue;
            }
            try {
              const chunk = JSON.parse(data);
              const delta = chunk?.choices?.[0]?.delta?.content;
              if (delta) {
                if (!sentCreated) {
                  const created = JSON.stringify({
                    type: 'response.created',
                    response: { id, object: 'response', model: chatReq.model, output: [{ id: `${id}_msg`, type: 'message', role: 'assistant', content: [] }] }
                  });
                  controller.enqueue(encoder.encode(`data: ${created}\n\n`));
                  sentCreated = true;
                }
                fullText += delta;
                const event = JSON.stringify({
                  type: 'response.output_text.delta',
                  item_id: `${id}_msg`, output_index: 0, content_index: 0, delta
                });
                controller.enqueue(encoder.encode(`data: ${event}\n\n`));
              }
            } catch {}
          }
        },
      });

      return new Response(stream, {
        headers: {
          'Content-Type': 'text/event-stream',
          'Cache-Control': 'no-cache',
        },
      });
    } else {
      const chatData = await chatResp.json() as any;
      const content = chatData.choices?.[0]?.message?.content ?? '';
      const id = `resp_${crypto.randomUUID().slice(0, 24)}`;
      
      return Response.json({
        id,
        object: 'response',
        model: chatReq.model,
        output: [{
          id: `${id}_msg`,
          type: 'message',
          role: 'assistant',
          content: [{ type: 'output_text', text: content }],
        }],
        usage: chatData.usage ?? { input_tokens: 0, output_tokens: 0 },
      });
    }
  },
});

console.log(`responses-proxy: listening on :${PORT}, upstream: ${UPSTREAM}`);
