import useRequestSignature from "@/hooks/useRequestSignature";
import useRequestTransactionSignature from "@/hooks/useRequestTransactionSignature";
import * as Popover from "@radix-ui/react-popover";
import { Button } from "./ui/button";
import { cn } from "@/lib/utils";
import SignatureRequestDialog from "./signature-request-dialog";
import SignTransactionRequestDialog from "./sign-transaction-request-dialog";
import InstallMetaMaskSnapButton from "./install-metamask-snap-button";
import { KeyringAccount, KeyringRequest, KeyringSnapRpcClient } from "@metamask/keyring-api";
import { env } from "@/env";
import { useQueries, useQuery } from "@tanstack/react-query";
import { ethers } from "ethers";
import { TypedDataUtils, SignTypedDataVersion, TypedMessage } from "@metamask/eth-sig-util";
import { MetadataEthereum } from "warden-protocol-wardenprotocol-client-ts/lib/warden.warden/module";
import { useMetaMask } from "@/def-hooks/useMetaMask";

async function buildSignTransaction(data: {
  chainId: string,
  data: string;
  from: string;
  gasLimit: string;
  maxFeePerGas: string;
  maxPriorityFeePerGas: string;
  nonce: string;
  to: string;
  type: string;
  value: string;
}) {
  return ethers.Transaction.from({
    chainId: data.chainId,
    data: data.data,
    gasLimit: data.gasLimit,
    maxFeePerGas: data.maxFeePerGas,
    maxPriorityFeePerGas: data.maxPriorityFeePerGas,
    nonce: ethers.getNumber(data.nonce),
    to: data.to,
    type: ethers.getNumber(data.type),
    value: data.value,
  });
}

function splitRSV(signature: Uint8Array) {
  return {
    r: ethers.hexlify(signature.slice(0, 32)),
    s: ethers.hexlify(signature.slice(32, 64)),
    v: ethers.hexlify(signature.slice(64, 65)),
  };
}

export function MetaMaskRequests() {
  const {
    state: reqSignatureState,
    error: reqSignatureError,
    requestSignature,
    reset: resetReqSignature,
  } = useRequestSignature();
  const {
    state: reqTxSignatureState,
    error: reqTxSignatureError,
    requestTransactionSignature,
    reset: resetReqTxSignature,
  } = useRequestTransactionSignature();
  const { installedSnap } = useMetaMask();

  const keyringSnapClient = new KeyringSnapRpcClient(env.snapOrigin, window.ethereum);
  const requestsQ = useQuery(["metamask-keyring-requests"], () => keyringSnapClient.listRequests(), {
    refetchInterval: 1000,
    enabled: !!installedSnap,
  });

  const accountsQ = useQueries({
    queries: requestsQ.data?.map((req) => ({
      queryKey: ["metamask-keyring-account", req.account],
      queryFn: () => keyringSnapClient.getAccount(req.account),
      refetchInterval: Infinity,
    })) ?? [],
  });
  const accountsQLoading = accountsQ.some((q) => q.isLoading);

  const accounts = accountsQ.reduce((acc, q) => {
    if (q.data) {
      acc[q.data.id] = q.data;
    }
    return acc;
  }, {} as Record<string, KeyringAccount>);

  const handleApproveRequest = async (req: KeyringRequest) => {
    const account = await keyringSnapClient.getAccount(req.account);
    const keyId = parseInt(account.options.keyId?.valueOf() as string, 10);
    if (!keyId || isNaN(keyId)) {
      throw new Error("Account has no keyId");
    }
    switch (req.request.method) {
      case "personal_sign": {
        if (!(req.request.params instanceof Array) || req.request.params?.length !== 2) {
          throw new Error("wrong params length");
        }
        const msgHex = req.request.params?.[0];
        if (!msgHex) {
          throw new Error("Request has no message");
        }
        const msg = ethers.hashMessage(ethers.getBytes(msgHex as string));
        const signature = await requestSignature(keyId, ethers.getBytes(msg));
        if (!signature) {
          throw new Error("Something went wrong waiting for signature request to complete");
        }
        await keyringSnapClient.approveRequest(req.id, { result: ethers.hexlify(signature) });
        break;
      }
      case "eth_signTransaction": {
        if (!(req.request.params instanceof Array) || req.request.params?.length !== 1) {
          throw new Error("wrong params length");
        }
        const txParam = req.request.params[0]?.valueOf() as any;
        const tx = await buildSignTransaction(txParam);
        const signature = await requestTransactionSignature(keyId, ethers.getBytes(tx.unsignedSerialized), {
          typeUrl:
            "/warden.warden.MetadataEthereum",
          value: MetadataEthereum.encode(
            {
              chainId: ethers.getNumber(txParam.chainId),
            }
          ).finish(),
        }
        );
        if (!signature) {
          throw new Error("Something went wrong waiting for signature request to complete");
        }

        tx.signature = ethers.hexlify(signature);

        await keyringSnapClient.approveRequest(req.id, {
          result: splitRSV(ethers.getBytes(signature))
        });

        break;
      }
      case "eth_signTypedData_v4": {
        if (!(req.request.params instanceof Array) || req.request.params?.length !== 2) {
          throw new Error("wrong params length");
        }
        const data = req.request.params[1]?.valueOf() as TypedMessage<any>;
        const toSign = TypedDataUtils.eip712Hash(data, SignTypedDataVersion.V4);

        const signature = await requestSignature(keyId, ethers.getBytes(toSign));
        if (!signature) {
          throw new Error("Something went wrong waiting for signature request to complete");
        }

        await keyringSnapClient.approveRequest(req.id, { result: ethers.hexlify(signature) });
        break;
      }
    }
  };

  const handleRejectRequest = async (req: KeyringRequest) => {
    await keyringSnapClient.rejectRequest(req.id);
  }

  return (
    <Popover.Root modal={true}>
      <Popover.Trigger asChild>
        <Button
          variant="ghost"
          size="icon"
          aria-label="Update dimensions"
          className={cn(
            "h-16 w-16 rounded-none border-l hover:bg-muted hover:border-b-accent hover:border-b-2"
          )}
        >
          <span>M</span>
        </Button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content className="bg-card border border-t-0 w-96 rounded-b-lg max-h-[calc(100vh-64px)] overflow-scroll no-scrollbar">
          <div className="p-4 flex flex-col space-y-4">
            <SignatureRequestDialog
              state={reqSignatureState}
              error={reqSignatureError}
              reset={resetReqSignature}
            />
            <SignTransactionRequestDialog
              state={reqTxSignatureState}
              error={reqTxSignatureError}
              reset={resetReqTxSignature}
            />

            <InstallMetaMaskSnapButton />

            {
              requestsQ.isLoading ? (
                <div>Loading...</div>
              ) : requestsQ.isError ? (
                <div>Error: {requestsQ.error?.toString()}</div>
              ) : requestsQ.data?.length === 0 ? (
                <div>No pending requests</div>
              ) : requestsQ.data?.map((req) => (
                <div key={req.id}>
                  <p>{
                    accounts[req.account]?.address
                      ? `For ${accounts[req.account].address}`
                      : accountsQLoading
                        ? "Loading info from MetaMask"
                        : "Error fetching account details from MetaMask"
                  }</p>
                  <p>{req.request.method}</p>
                  <Button size="sm" onClick={() => handleApproveRequest(req)}>
                    Approve
                  </Button>
                  <Button size="sm" variant="destructive" onClick={() => handleRejectRequest(req)}>
                    Reject
                  </Button>
                </div>
              ))
            }
          </div>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
